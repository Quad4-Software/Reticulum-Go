// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public final class Link {
    private var handle: UInt64

    init(handle: UInt64) {
        self.handle = handle
    }

    deinit {
        close()
    }

    public var rawHandle: UInt64 { handle }

    public static func open(node: Node, destHash: Data) throws -> Link {
        guard destHash.count == HASH_LEN else { throw RNSError.invalidArgument }
        let h = destHash.withUnsafeBytes { raw in
            rns_link_open(node.rawHandle, raw.bindMemory(to: UInt8.self).baseAddress)
        }
        guard h != 0 else { throw RNSError.internalError }
        return Link(handle: h)
    }

    public func send(_ data: Data) throws {
        guard !data.isEmpty else { throw RNSError.invalidArgument }
        try data.withUnsafeBytes { raw in
            try check(rns_link_send(
                handle,
                raw.bindMemory(to: UInt8.self).baseAddress,
                data.count
            ))
        }
    }

    public func sendResource(_ data: Data, name: String = "") throws {
        try data.withUnsafeBytes { raw in
            try name.withCString { namePtr in
                try check(rns_link_send_resource(
                    handle,
                    raw.bindMemory(to: UInt8.self).baseAddress,
                    data.count,
                    name.isEmpty ? nil : namePtr
                ))
            }
        }
    }

    public func id() throws -> Data {
        var out = [UInt8](repeating: 0, count: HASH_LEN)
        var written: Int = 0
        try out.withUnsafeMutableBufferPointer { ptr in
            try check(rns_link_id(handle, ptr.baseAddress, HASH_LEN, &written))
        }
        guard written == HASH_LEN else { throw RNSError.truncated }
        return Data(out)
    }

    public func request(node: Node, path: String, data: Data = Data(), timeoutMs: Int32 = 0) throws -> Data {
        guard !path.isEmpty else { throw RNSError.invalidArgument }
        var requestId = [UInt8](repeating: 0, count: HASH_LEN)
        var written: Int = 0
        try path.withCString { pathPtr in
            try data.withUnsafeBytes { raw in
                try requestId.withUnsafeMutableBufferPointer { out in
                    try check(rns_link_request(
                        node.rawHandle,
                        handle,
                        pathPtr,
                        raw.bindMemory(to: UInt8.self).baseAddress,
                        data.count,
                        timeoutMs,
                        out.baseAddress,
                        HASH_LEN,
                        &written
                    ))
                }
            }
        }
        guard written == HASH_LEN else { throw RNSError.truncated }
        return Data(requestId)
    }

    public func close() {
        if handle != 0 {
            _ = rns_link_close(handle)
            handle = 0
        }
    }
}

public func requestRespond(node: Node, requestId: Data, data: Data) throws {
    guard !requestId.isEmpty else { throw RNSError.invalidArgument }
    try requestId.withUnsafeBytes { req in
        try data.withUnsafeBytes { payload in
            try check(rns_request_respond(
                node.rawHandle,
                req.bindMemory(to: UInt8.self).baseAddress,
                requestId.count,
                payload.bindMemory(to: UInt8.self).baseAddress,
                data.count
            ))
        }
    }
}

public func requestRespondFile(node: Node, requestId: Data, filename: String, data: Data) throws {
    guard !requestId.isEmpty, !filename.isEmpty else { throw RNSError.invalidArgument }
    try filename.withCString { namePtr in
        try requestId.withUnsafeBytes { req in
            try data.withUnsafeBytes { payload in
                try check(rns_request_respond_file(
                    node.rawHandle,
                    req.bindMemory(to: UInt8.self).baseAddress,
                    requestId.count,
                    namePtr,
                    payload.bindMemory(to: UInt8.self).baseAddress,
                    data.count
                ))
            }
        }
    }
}
