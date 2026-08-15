// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style pageserver over Java librns bindings.
// Usage: java Main -c config [-i identity] [-a announce_sec] [-p page_file]

import io.quad4.rns.Destination;
import io.quad4.rns.Event;
import io.quad4.rns.Identity;
import io.quad4.rns.Link;
import io.quad4.rns.Node;
import io.quad4.rns.Rns;
import io.quad4.rns.RnsException;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.Arrays;

public final class Main {
    private static final int DEFAULT_ANNOUNCE_SEC = 900;
    private static final String DEFAULT_PAGE_PATH = "/page/index.mu";
    private static final String DEFAULT_FILE_PATH = "/file/test.txt";
    private static final String DEFAULT_PAGE_FILE = "pages/index.mu";
    private static final String DEFAULT_FILE_FILE = "files/test.txt";
    private static final String DEFAULT_IDENTITY_PATH = "identity";
    private static final int REQ_DATA_CAP = 64 * 1024;
    private static final String FALLBACK_PAGE =
            "> Java pageserver\n\nlibrns via Reticulum-Go\n\nFallback page (file not found).\n\n";
    private static final String FALLBACK_FILE = "Test file from Reticulum-Go node!\n";

    public static void main(String[] args) throws Exception {
        String configPath = null;
        String identityPath = DEFAULT_IDENTITY_PATH;
        int announceSec = DEFAULT_ANNOUNCE_SEC;
        String pageFile = DEFAULT_PAGE_FILE;
        String fileFile = DEFAULT_FILE_FILE;
        String requestPath = DEFAULT_PAGE_PATH;
        for (int i = 0; i < args.length; i++) {
            if ("-c".equals(args[i]) && i + 1 < args.length) {
                configPath = args[++i];
            } else if ("-i".equals(args[i]) && i + 1 < args.length) {
                identityPath = args[++i];
            } else if ("-a".equals(args[i]) && i + 1 < args.length) {
                announceSec = Integer.parseInt(args[++i]);
            } else if ("-p".equals(args[i]) && i + 1 < args.length) {
                pageFile = args[++i];
            } else if ("-f".equals(args[i]) && i + 1 < args.length) {
                fileFile = args[++i];
            } else if ("-P".equals(args[i]) && i + 1 < args.length) {
                requestPath = args[++i];
            } else {
                die("unexpected argument: " + args[i]);
            }
        }
        if (configPath == null || announceSec < 0) {
            die("usage: java Main -c config [options]");
        }
        if (!Rns.API_VERSION.equals(Rns.version())) {
            die("librns version mismatch: got " + Rns.version());
        }

        byte[] pageBody = loadBytes(pageFile, FALLBACK_PAGE);
        byte[] fileBody = loadBytes(fileFile, FALLBACK_FILE);

        try (Node node = Node.create(configPath);
                Identity identity = loadOrCreateIdentity(identityPath);
                Destination dest =
                        Destination.create(node, null, "nomadnetwork", Arrays.asList("node"), true)) {
            node.setIdentity(identity);
            node.start();
            dest.registerRequestHandler(requestPath);
            dest.registerRequestHandler(DEFAULT_FILE_PATH);

            String destHex = Rns.hashToHex(dest.hash());
            System.out.println("DEST_HASH=" + destHex);
            System.out.println("REQUEST_PATH=" + requestPath);
            System.out.println("FILE_PATH=" + DEFAULT_FILE_PATH);
            System.err.println("librns " + Rns.version() + " pageserver listening as nomadnetwork.node");

            byte[] appData = "librns-java-pageserver".getBytes(StandardCharsets.UTF_8);
            try {
                dest.announce(appData);
                System.err.println("announce sent");
            } catch (RnsException e) {
                printLastError("destination.announce failed");
            }

            long lastAnnounce = System.currentTimeMillis();
            while (true) {
                if (announceSec > 0 && System.currentTimeMillis() - lastAnnounce >= announceSec * 1000L) {
                    try {
                        dest.announce(appData);
                        System.err.println("announce refreshed");
                    } catch (RnsException ignored) {
                        // continue
                    }
                    lastAnnounce = System.currentTimeMillis();
                }

                try {
                    Event ev = Event.poll(node, 200, REQ_DATA_CAP);
                    if (ev.kind() == Event.LINK_ESTABLISHED) {
                        System.err.println("inbound link established");
                    } else if (ev.kind() == Event.LINK_CLOSED) {
                        System.err.println("link closed");
                    } else if (ev.kind() == Event.REQUEST_INCOMING) {
                        String path = ev.path();
                        System.err.println("request incoming path=" + path);
                        byte[] reqId = ev.requestId();
                        if (requestPath.equals(path)) {
                            try {
                                Link.requestRespond(node, reqId, pageBody);
                                System.err.println("served " + requestPath + " (" + pageBody.length + " bytes)");
                            } catch (RnsException e) {
                                printLastError("request_respond failed");
                            }
                        } else if (DEFAULT_FILE_PATH.equals(path)) {
                            try {
                                Link.requestRespondFile(node, reqId, "test.txt", fileBody);
                                System.err.println(
                                        "served " + DEFAULT_FILE_PATH + " (" + fileBody.length + " bytes)");
                            } catch (RnsException e) {
                                printLastError("request_respond_file failed");
                            }
                        } else {
                            try {
                                Link.requestRespond(node, reqId, "page not found\n".getBytes(StandardCharsets.UTF_8));
                            } catch (RnsException e) {
                                printLastError("request_respond failed");
                            }
                        }
                    }
                } catch (RnsException e) {
                    if (e.getCode() != RnsException.TIMEOUT) {
                        printLastError("Event.poll failed");
                        System.exit(1);
                    }
                }
            }
        }
    }

    private static byte[] loadBytes(String path, String fallback) {
        try {
            return Files.readAllBytes(Paths.get(path));
        } catch (IOException e) {
            System.err.println("warning: could not read " + path + ", using built-in content");
            return fallback.getBytes(StandardCharsets.UTF_8);
        }
    }

    private static Identity loadOrCreateIdentity(String path) {
        try {
            Identity identity = Identity.load(path);
            System.err.println("loaded identity from " + path);
            return identity;
        } catch (RnsException e) {
            Identity identity = Identity.generate();
            identity.save(path);
            System.err.println("created and saved identity to " + path);
            return identity;
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
