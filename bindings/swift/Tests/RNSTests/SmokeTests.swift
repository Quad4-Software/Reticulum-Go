// SPDX-License-Identifier: LicenseRef-Reticulum
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

    func testAddedABISurface() throws {
        let node = try Node.create()
        defer { node.close() }
        XCTAssertThrowsError(try node.reloadConfig()) { error in
            XCTAssertEqual(error as? RNSError, .invalidArgument)
        }
        let configPath = NSTemporaryDirectory() + "/rns-swift-reload.conf"
        try Data("[reticulum]\nenable_transport = no\nin_memory_storage = yes\n".utf8).write(to: URL(fileURLWithPath: configPath))
        let configured = try Node.create(configPath: configPath)
        try configured.reloadConfig()
        configured.close()
        try node.refreshPaths()
        let unknown = Data(repeating: 0, count: HASH_LEN)
        XCTAssertThrowsError(try destinationEncrypt(destHash: unknown, plaintext: Data("x".utf8))) { error in
            XCTAssertEqual(error as? RNSError, .notFound)
        }
        XCTAssertThrowsError(try node.packetSend(destHash: unknown, plaintext: Data("x".utf8)))
        XCTAssertThrowsError(try Link.fromId(node: node, linkId: unknown)) { error in
            XCTAssertEqual(error as? RNSError, .notFound)
        }
        try node.setEventCallback { _ in }
        try node.setEventCallback(nil)

        let identity = try Identity.generate()
        defer { identity.close() }
        let dest = try Destination.create(node: node, identity: identity, appName: "swifttest", aspects: [], acceptsLinks: false)
        defer { dest.close() }
        try dest.setProofStrategy(0)
        XCTAssertThrowsError(try dest.setProofStrategy(255))

        let path = NSTemporaryDirectory() + "/rns-swift-test.txt"
        try Data("signed".utf8).write(to: URL(fileURLWithPath: path))
        let sig = try rsgSignFile(identity: identity, path: path)
        try rsgVerifyFile(rsg: sig, path: path)
        try Data("tampered".utf8).write(to: URL(fileURLWithPath: path))
        XCTAssertThrowsError(try rsgVerifyFile(rsg: sig, path: path))
    }
}
