// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public struct PathInfo {
    public let hash: Data
    public let via: Data
    public let hops: UInt8
    public let iface: String
    public let timestamp: Double
    public let expires: Double
}

public func pathRequest(node: Node, destHash: Data) throws {
    guard destHash.count == HASH_LEN else { throw RNSError.invalidArgument }
    try destHash.withUnsafeBytes { raw in
        try check(rns_path_request(node.rawHandle, raw.bindMemory(to: UInt8.self).baseAddress))
    }
}

public func pathTable(node: Node, capacity: Int = 256, maxHops: Int32 = -1) throws -> [PathInfo] {
    guard capacity > 0 else { throw RNSError.invalidArgument }
    var entries = [rns_path_entry](repeating: rns_path_entry(), count: capacity)
    var written: Int = 0
    let code = entries.withUnsafeMutableBufferPointer { ptr in
        rns_path_table(node.rawHandle, ptr.baseAddress, capacity, &written, maxHops)
    }
    if code != RNS_OK && code != RNS_ERR_TRUNCATED {
        try check(code)
    }
    var out: [PathInfo] = []
    out.reserveCapacity(written)
    for i in 0..<written {
        let e = entries[i]
        out.append(PathInfo(
            hash: withUnsafeBytes(of: e.hash) { Data($0.prefix(e.hash_len)) },
            via: withUnsafeBytes(of: e.via) { Data($0.prefix(e.via_len)) },
            hops: e.hops,
            iface: withUnsafeBytes(of: e.iface) { String(cString: $0.bindMemory(to: CChar.self).baseAddress!) },
            timestamp: e.timestamp,
            expires: e.expires
        ))
    }
    return out
}

public func pathKnown(node: Node, destHash: Data) -> Bool {
    guard destHash.count == HASH_LEN else { return false }
    guard let table = try? pathTable(node: node) else { return false }
    return table.contains { $0.hash == destHash }
}
