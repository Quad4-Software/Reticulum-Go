// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import XCTest
import RNS
import Foundation

final class SmokeTests: XCTestCase {
    func testVersion() {
        XCTAssertEqual(version(), API_VERSION)
    }

    func testNodeLifecycle() throws {
        let node = try Node.create()
        defer { node.close() }
        try node.start()
        do {
            _ = try Event.poll(node: node, timeoutMs: 10)
            XCTFail("expected timeout")
        } catch let err as RNSError {
            XCTAssertEqual(err, .timeout)
        }
        try node.stop()
    }

    func testIdentitySignVerify() throws {
        let identity = try Identity.generate()
        defer { identity.close() }
        let hexHash = try identity.hashHex()
        XCTAssertEqual(hexHash.count, 32)
        XCTAssertEqual(try identity.hashBytes().count, 16)
        let msg = Data("hello-rns".utf8)
        let sig = try identity.sign(msg)
        try identity.verify(msg, signature: sig)
        let pub = try identity.publicKey()
        let onlyPub = try Identity.fromPublicKey(pub)
        defer { onlyPub.close() }
        try onlyPub.verify(msg, signature: sig)
    }
}
