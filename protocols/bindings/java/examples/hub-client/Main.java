// SPDX-License-Identifier: LicenseRef-Reticulum

import io.quad4.rrc.Client;
import io.quad4.rrc.Event;
import io.quad4.rrc.Hub;
import io.quad4.rrc.Identity;
import io.quad4.rrc.Node;
import io.quad4.rrc.RrcException;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Main {
    private static final String HUB_LOCAL = "127.0.0.1:42560";
    private static final String HUB_PEER = "127.0.0.1:42561";
    private static final String CLI_LOCAL = "127.0.0.1:42561";
    private static final String CLI_PEER = "127.0.0.1:42560";

    public static void main(String[] args) throws Exception {
        try (Node hubNode = Node.create(); Node cliNode = Node.create()) {
            hubNode.addUdpInterface("H1", HUB_LOCAL, HUB_PEER);
            cliNode.addUdpInterface("C1", CLI_LOCAL, CLI_PEER);

            try (Identity idH = Identity.generate(); Identity idC = Identity.generate()) {
                hubNode.setIdentity(idH);
                cliNode.setIdentity(idC);
                hubNode.start();
                cliNode.start();

                try (Hub hub = Hub.create(hubNode.handle(), idH.handle(), "java-hub", "1.0")) {
                    hub.start();
                    hub.announce();
                    byte[] hubHash = hub.hashBytes();
                    idH.seedDestination(hubHash);

                    long deadline = System.currentTimeMillis() + 15000;
                    while (System.currentTimeMillis() < deadline) {
                        if (cliNode.hasPath(hubHash)) {
                            break;
                        }
                        Thread.sleep(50);
                    }
                    if (!cliNode.hasPath(hubHash)) {
                        fail("path timeout");
                    }

                    try (Client client = Client.dial(
                        cliNode.handle(),
                        idC.handle(),
                        hubHash,
                        "alice",
                        "java-client",
                        "1.0",
                        15000
                    )) {
                        client.join("#lobby");
                        boolean joined = false;
                        long joinEnd = System.currentTimeMillis() + 10000;
                        while (System.currentTimeMillis() < joinEnd) {
                            try {
                                Event ev = client.eventPoll(500);
                                if (ev.kind == RrcLibrary.RRC_EV_JOINED) {
                                    joined = true;
                                    break;
                                }
                            } catch (RrcException ex) {
                                if (ex.getCode() != RrcException.TIMEOUT) {
                                    throw ex;
                                }
                            }
                        }
                        if (!joined) {
                            fail("join timeout");
                        }

                        String want = "hello from java hub-client";
                        client.sendMsg("#lobby", want);

                        long msgEnd = System.currentTimeMillis() + 10000;
                        while (System.currentTimeMillis() < msgEnd) {
                            try {
                                Event ev = hub.eventPoll(500);
                                if (ev.kind == RrcLibrary.RRC_EV_MSG && want.equals(ev.body)) {
                                    System.out.println("java-hub-client ok");
                                    return;
                                }
                            } catch (RrcException ex) {
                                if (ex.getCode() != RrcException.TIMEOUT) {
                                    throw ex;
                                }
                            }
                        }
                        fail("hub did not receive message");
                    }
                }
            }
        }
    }

    private static void fail(String msg) {
        System.err.println(msg);
        System.exit(1);
    }
}
