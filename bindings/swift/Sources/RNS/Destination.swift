// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public final class Destination {
    private var handle: UInt64

    init(handle: UInt64) {
        self.handle = handle
    }

    deinit {
        close()
    }

    public var rawHandle: UInt64 { handle }

    public static func create(
        node: Node,
        identity: Identity?,
        appName: String,
        aspects: [String],
        acceptsLinks: Bool
    ) throws -> Destination {
        guard !appName.isEmpty else { throw RNSError.invalidArgument }

        var cAspects = aspects.map { strdup($0) as UnsafeMutablePointer<CChar>? }
        defer {
            for ptr in cAspects {
                if let ptr { free(ptr) }
            }
        }

        let created: UInt64 = cAspects.withUnsafeMutableBufferPointer { buf in
            appName.withCString { appPtr in
                if aspects.isEmpty {
                    return rns_destination_create(
                        node.rawHandle,
                        identity?.rawHandle ?? 0,
                        appPtr,
                        nil,
                        0,
                        acceptsLinks ? 1 : 0
                    )
                }
                return buf.baseAddress!.withMemoryRebound(to: UnsafePointer<CChar>?.self, capacity: aspects.count) { rebound in
                    rns_destination_create(
                        node.rawHandle,
                        identity?.rawHandle ?? 0,
                        appPtr,
                        UnsafePointer(rebound),
                        aspects.count,
                        acceptsLinks ? 1 : 0
                    )
                }
            }
        }
        guard created != 0 else { throw RNSError.internalError }
        return Destination(handle: created)
    }

    public func enableRatchets(path: String? = nil) throws {
        if let path {
            try path.withCString { cPath in
                try check(rns_destination_enable_ratchets(handle, cPath))
            }
        } else {
            try check(rns_destination_enable_ratchets(handle, nil))
        }
    }

    public func enforceRatchets() throws {
        try check(rns_destination_enforce_ratchets(handle))
    }

    public func setProofStrategy(_ strategy: Int32) throws {
        try check(rns_destination_set_proof_strategy(handle, strategy))
    }

    public func announce(appData: Data = Data()) throws {
        try appData.withUnsafeBytes { raw in
            try check(rns_destination_announce(
                handle,
                raw.bindMemory(to: UInt8.self).baseAddress,
                appData.count
            ))
        }
    }

    public func hash() throws -> Data {
        var out = [UInt8](repeating: 0, count: HASH_LEN)
        var written: Int = 0
        try out.withUnsafeMutableBufferPointer { ptr in
            try check(rns_destination_hash(handle, ptr.baseAddress, HASH_LEN, &written))
        }
        guard written == HASH_LEN else { throw RNSError.truncated }
        return Data(out)
    }

    public func registerRequestHandler(path: String) throws {
        guard !path.isEmpty else { throw RNSError.invalidArgument }
        try path.withCString { try check(rns_destination_register_request_handler(handle, $0)) }
    }

    public func close() {
        if handle != 0 {
            _ = rns_destination_destroy(handle)
            handle = 0
        }
    }
}

public func destinationEncrypt(destHash: Data, plaintext: Data) throws -> Data {
    guard destHash.count == HASH_LEN, !plaintext.isEmpty else { throw RNSError.invalidArgument }
    var needed: Int = 0
    let probe: Int32 = destHash.withUnsafeBytes { dest in
        plaintext.withUnsafeBytes { payload in
            rns_destination_encrypt(
                dest.bindMemory(to: UInt8.self).baseAddress,
                payload.bindMemory(to: UInt8.self).baseAddress,
                plaintext.count,
                nil,
                0,
                &needed
            )
        }
    }
    if probe != RNS_OK && probe != RNS_ERR_TRUNCATED {
        try check(probe)
    }
    var out = [UInt8](repeating: 0, count: needed)
    var written: Int = 0
    try destHash.withUnsafeBytes { dest in
        try plaintext.withUnsafeBytes { payload in
            try out.withUnsafeMutableBufferPointer { buf in
                try check(rns_destination_encrypt(
                    dest.bindMemory(to: UInt8.self).baseAddress,
                    payload.bindMemory(to: UInt8.self).baseAddress,
                    plaintext.count,
                    buf.baseAddress,
                    needed,
                    &written
                ))
            }
        }
    }
    return Data(out.prefix(written))
}
