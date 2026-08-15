// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style page fetch over Java librns bindings.
// Usage: java Main -c config [-t timeout_sec] <dest_hash>:<page_path>

import io.quad4.rns.Event;
import io.quad4.rns.Identity;
import io.quad4.rns.Link;
import io.quad4.rns.Node;
import io.quad4.rns.Path;
import io.quad4.rns.Rns;
import io.quad4.rns.RnsException;
import java.nio.charset.StandardCharsets;

public final class Main {
    private static final int PAGE_BUF_CAP = 512 * 1024;
    private static final int DEFAULT_TIMEOUT_SEC = 60;
    private static final long PATH_RETRY_MS = 2000;

    public static void main(String[] args) {
        String configPath = null;
        int timeoutSec = DEFAULT_TIMEOUT_SEC;
        String target = null;
        for (int i = 0; i < args.length; i++) {
            if ("-c".equals(args[i]) && i + 1 < args.length) {
                configPath = args[++i];
            } else if ("-t".equals(args[i]) && i + 1 < args.length) {
                timeoutSec = Integer.parseInt(args[++i]);
            } else if (target == null) {
                target = args[i];
            } else {
                die("unexpected argument: " + args[i]);
            }
        }
        if (configPath == null || target == null || timeoutSec <= 0) {
            die("usage: java Main -c config [-t timeout] <dest_hash>:<page_path>");
        }

        if (!Rns.API_VERSION.equals(Rns.version())) {
            die("librns version mismatch: got " + Rns.version());
        }

        int colon = target.indexOf(':');
        if (colon <= 0 || colon == target.length() - 1) {
            die("target must be <32-hex-dest>:<page_path>");
        }
        byte[] destHash = Rns.hexToHash(target.substring(0, colon));
        String pagePath = target.substring(colon + 1);
        String destHex = Rns.hashToHex(destHash);

        try (Node node = Node.create(configPath);
                Identity identity = Identity.generate()) {
            node.setIdentity(identity);
            node.start();
            System.out.println("librns " + Rns.version() + " fetching " + pagePath + " from " + destHex);

            long deadline = System.currentTimeMillis() + timeoutSec * 1000L;
            long lastPathReq = 0;
            boolean needPathReq = true;
            boolean sawAnnounce = false;
            Link link = null;

            while (System.currentTimeMillis() < deadline && link == null) {
                long now = System.currentTimeMillis();
                if (needPathReq || now - lastPathReq >= PATH_RETRY_MS) {
                    try {
                        Path.request(node, destHash);
                    } catch (RnsException e) {
                        printLastError("path_request failed");
                    }
                    lastPathReq = now;
                    needPathReq = false;
                    if (Path.known(node, destHash)) {
                        System.err.println("path known, waiting for destination identity announce");
                    } else {
                        System.err.println("requesting path to " + destHex);
                    }
                }

                try {
                    Event ev = Event.poll(node, 200, PAGE_BUF_CAP);
                    if (ev.kind() == Event.ANNOUNCE && Rns.hashEq(ev.destinationHash(), destHash)) {
                        sawAnnounce = true;
                        System.err.println("announce for target (hops=" + ev.hops() + ")");
                        try {
                            link = Link.open(node, destHash);
                        } catch (RnsException e) {
                            printLastError("Link.open after announce");
                        }
                    } else if (ev.kind() == Event.LINK_FAILED) {
                        System.err.println("link failed while opening: " + ev.errorMessage());
                    }
                } catch (RnsException e) {
                    if (e.getCode() == RnsException.TIMEOUT) {
                        if (sawAnnounce || Path.known(node, destHash)) {
                            try {
                                link = Link.open(node, destHash);
                            } catch (RnsException ignored) {
                                // retry
                            }
                        }
                    } else {
                        printLastError("Event.poll failed");
                        System.exit(1);
                    }
                }
            }

            if (link == null) {
                die("timed out before link open");
            }

            try (Link establishedLink = link) {
                boolean established = false;
                while (System.currentTimeMillis() < deadline && !established) {
                    try {
                        Event ev = Event.poll(node, 500, PAGE_BUF_CAP);
                        if (ev.kind() == Event.LINK_ESTABLISHED) {
                            established = true;
                            System.err.println("link established");
                        } else if (ev.kind() == Event.LINK_FAILED) {
                            die("link establishment failed: " + ev.errorMessage());
                        } else if (ev.kind() == Event.LINK_CLOSED) {
                            die("link closed before establish");
                        }
                    } catch (RnsException e) {
                        if (e.getCode() != RnsException.TIMEOUT) {
                            printLastError("Event.poll failed");
                            System.exit(1);
                        }
                    }
                }
                if (!established) {
                    die("timed out waiting for link establishment");
                }

                int remainingMs = (int) Math.max(deadline - System.currentTimeMillis(), 1000);
                establishedLink.request(node, pagePath, new byte[0], remainingMs);
                System.err.println("request sent for " + pagePath);

                while (System.currentTimeMillis() < deadline) {
                    try {
                        Event ev = Event.poll(node, 500, PAGE_BUF_CAP);
                        if (ev.kind() == Event.REQUEST_RESPONSE) {
                            byte[] data = ev.appData();
                            System.out.println("\n=== Page Content (" + data.length + " bytes) ===");
                            System.out.print(new String(data, StandardCharsets.UTF_8));
                            if (data.length == 0 || data[data.length - 1] != '\n') {
                                System.out.println();
                            }
                            if (ev.appDataTruncated()) {
                                System.err.println("warning: response truncated");
                            }
                            System.out.println("=== End of Page ===");
                            return;
                        } else if (ev.kind() == Event.REQUEST_FAILED) {
                            die("request failed: " + ev.errorMessage());
                        } else if (ev.kind() == Event.LINK_CLOSED) {
                            die("link closed before response");
                        }
                    } catch (RnsException e) {
                        if (e.getCode() != RnsException.TIMEOUT) {
                            printLastError("Event.poll failed");
                            System.exit(1);
                        }
                    }
                }
                die("timed out waiting for page response");
            }
        }
    }

    private static void die(String msg) {
        System.err.println(msg);
        System.exit(1);
    }

    private static void printLastError(String what) {
        String msg = Rns.lastError();
        if (msg.isEmpty()) {
            System.err.println(what);
        } else {
            System.err.println(what + ": " + msg);
        }
    }
}
