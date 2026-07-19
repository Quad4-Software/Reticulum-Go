// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public final class Identity {
    private var handle: UInt64

    init(handle: UInt64) {
        self.handle = handle
    }

    deinit {
        close()
    }

    public var rawHandle: UInt64 { handle }

    public static func generate() throws -> Identity {
        let h = rns_identity_generate()
        guard h != 0 else { throw RNSError.internalError }
        return Identity(handle: h)
    }

    public static func load(path: String) throws -> Identity {
        guard !path.isEmpty else { throw RNSError.invalidArgument }
        let h = path.withCString { rns_identity_load($0) }
        guard h != 0 else { throw RNSError.io }
        return Identity(handle: h)
    }

    public static func fromPublicKey(_ pub: Data) throws -> Identity {
        guard !pub.isEmpty else { throw RNSError.invalidArgument }
        let h = pub.withUnsafeBytes { raw in
            rns_identity_from_public_key(raw.bindMemory(to: UInt8.self).baseAddress, pub.count)
        }
        guard h != 0 else { throw RNSError.invalidArgument }
        return Identity(handle: h)
    }

    public func save(path: String) throws {
        guard !path.isEmpty else { throw RNSError.invalidArgument }
        try path.withCString { try check(rns_identity_save(handle, $0)) }
    }

    public func hashHex() throws -> String {
        var buf = [CChar](repeating: 0, count: 64)
        var written: Int = 0
        try buf.withUnsafeMutableBufferPointer { ptr in
            try check(rns_identity_hash(handle, ptr.baseAddress, ptr.count, &written))
        }
        _ = written
        return String(cString: buf)
    }

    public func hashBytes() throws -> Data {
        var out = [UInt8](repeating: 0, count: HASH_LEN)
        var written: Int = 0
        try out.withUnsafeMutableBufferPointer { ptr in
            try check(rns_identity_hash_bytes(handle, ptr.baseAddress, HASH_LEN, &written))
        }
        guard written == HASH_LEN else { throw RNSError.truncated }
        return Data(out)
    }

    public func publicKey() throws -> Data {
        var out = [UInt8](repeating: 0, count: 64)
        var written: Int = 0
        try out.withUnsafeMutableBufferPointer { ptr in
            try check(rns_identity_public_key(handle, ptr.baseAddress, 64, &written))
        }
        return Data(out.prefix(written))
    }

    public func sign(_ data: Data) throws -> Data {
        var sig = [UInt8](repeating: 0, count: 64)
        var written: Int = 0
        try data.withUnsafeBytes { raw in
            try sig.withUnsafeMutableBufferPointer { out in
                try check(rns_identity_sign(
                    handle,
                    raw.bindMemory(to: UInt8.self).baseAddress,
                    data.count,
                    out.baseAddress,
                    64,
                    &written
                ))
            }
        }
        return Data(sig.prefix(written))
    }

    public func verify(_ data: Data, signature: Data) throws {
        try data.withUnsafeBytes { dRaw in
            try signature.withUnsafeBytes { sRaw in
                try check(rns_identity_verify(
                    handle,
                    dRaw.bindMemory(to: UInt8.self).baseAddress,
                    data.count,
                    sRaw.bindMemory(to: UInt8.self).baseAddress,
                    signature.count
                ))
            }
        }
    }

    public func close() {
        if handle != 0 {
            _ = rns_identity_destroy(handle)
            handle = 0
        }
    }
}
