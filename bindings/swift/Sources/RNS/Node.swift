// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

private final class EventCallbackBox {
    let closure: (Event) -> Void
    init(_ closure: @escaping (Event) -> Void) {
        self.closure = closure
    }
}

private let eventCallbackBridge: rns_event_callback = { raw, userData in
    guard let raw, let userData else { return }
    let box = Unmanaged<EventCallbackBox>.fromOpaque(userData).takeUnretainedValue()
    box.closure(Event.fromC(raw.pointee))
}

public final class Node {
    private var handle: UInt64
    private var eventCallbackBox: EventCallbackBox?
    private var eventCallbackUserData: UnsafeMutableRawPointer?

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

    public func reloadConfig() throws {
        try check(rns_node_reload_config(handle))
    }

    public func refreshPaths(destHashes: [Data] = []) throws {
        if destHashes.isEmpty {
            try check(rns_node_refresh_paths(handle, nil, 0))
            return
        }
        var flat = [UInt8]()
        flat.reserveCapacity(destHashes.count * HASH_LEN)
        for hash in destHashes {
            guard hash.count == HASH_LEN else { throw RNSError.invalidArgument }
            flat.append(contentsOf: hash)
        }
        try flat.withUnsafeBufferPointer { ptr in
            try check(rns_node_refresh_paths(handle, ptr.baseAddress, destHashes.count))
        }
    }

    public func packetSend(destHash: Data, plaintext: Data) throws {
        guard destHash.count == HASH_LEN, !plaintext.isEmpty else { throw RNSError.invalidArgument }
        try destHash.withUnsafeBytes { dest in
            try plaintext.withUnsafeBytes { payload in
                try check(rns_packet_send(
                    handle,
                    dest.bindMemory(to: UInt8.self).baseAddress,
                    payload.bindMemory(to: UInt8.self).baseAddress,
                    plaintext.count
                ))
            }
        }
    }

    public func setEventCallback(_ callback: ((Event) -> Void)?) throws {
        if eventCallbackUserData != nil {
            try check(rns_set_event_callback(handle, nil, nil))
            if let userData = eventCallbackUserData {
                Unmanaged<EventCallbackBox>.fromOpaque(userData).release()
            }
            eventCallbackUserData = nil
            eventCallbackBox = nil
        }
        guard let callback else { return }

        let box = EventCallbackBox(callback)
        eventCallbackBox = box
        let userData = Unmanaged.passRetained(box).toOpaque()
        do {
            try check(rns_set_event_callback(handle, eventCallbackBridge, userData))
            eventCallbackUserData = userData
        } catch {
            Unmanaged<EventCallbackBox>.fromOpaque(userData).release()
            eventCallbackBox = nil
            throw error
        }
    }

    public func close() {
        if handle != 0 {
            _ = rns_set_event_callback(handle, nil, nil)
            if let userData = eventCallbackUserData {
                Unmanaged<EventCallbackBox>.fromOpaque(userData).release()
            }
            eventCallbackUserData = nil
            eventCallbackBox = nil
            _ = rns_node_destroy(handle)
            handle = 0
        }
    }
}
