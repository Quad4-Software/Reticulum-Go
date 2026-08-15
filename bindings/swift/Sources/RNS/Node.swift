// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public final class Node {
    private var handle: UInt64

    init(handle: UInt64) {
        self.handle = handle
    }

    deinit {
        close()
    }

    public var rawHandle: UInt64 { handle }

    public static func create(configPath: String = "") throws -> Node {
        let h = configPath.withCString { rns_node_create($0) }
        guard h != 0 else { throw RNSError.internalError }
        return Node(handle: h)
    }

    public func start() throws {
        try check(rns_node_start(handle))
    }

    public func stop() throws {
        try check(rns_node_stop(handle))
    }

    public func setIdentity(_ identity: Identity) throws {
        try check(rns_node_set_identity(handle, identity.rawHandle))
    }

    public func pause() throws {
        try check(rns_node_pause(handle))
    }

    public func resume() throws {
        try check(rns_node_resume(handle))
    }

    public func close() {
        if handle != 0 {
            _ = rns_node_destroy(handle)
            handle = 0
        }
    }
}
