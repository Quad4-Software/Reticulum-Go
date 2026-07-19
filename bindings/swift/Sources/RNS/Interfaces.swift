// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public struct InterfaceInfo {
    public let name: String
    public let typeName: String
    public let online: Bool
    public let enabled: Bool
    public let rxBytes: UInt64
    public let txBytes: UInt64
    public let rxPackets: UInt64
    public let txPackets: UInt64
}

public func interfacesList(node: Node, capacity: Int = 32) throws -> [InterfaceInfo] {
    guard capacity > 0 else { throw RNSError.invalidArgument }
    var entries = [rns_interface_entry](repeating: rns_interface_entry(), count: capacity)
    var written: Int = 0
    let code = entries.withUnsafeMutableBufferPointer { ptr in
        rns_interfaces(node.rawHandle, ptr.baseAddress, capacity, &written)
    }
    if code != RNS_OK && code != RNS_ERR_TRUNCATED {
        try check(code)
    }
    var out: [InterfaceInfo] = []
    out.reserveCapacity(written)
    for i in 0..<written {
        let e = entries[i]
        out.append(InterfaceInfo(
            name: withUnsafeBytes(of: e.name) { raw in
                String(cString: raw.bindMemory(to: CChar.self).baseAddress!)
            },
            typeName: withUnsafeBytes(of: e.type_name) { raw in
                String(cString: raw.bindMemory(to: CChar.self).baseAddress!)
            },
            online: e.online != 0,
            enabled: e.enabled != 0,
            rxBytes: e.rx_bytes,
            txBytes: e.tx_bytes,
            rxPackets: e.rx_packets,
            txPackets: e.tx_packets
        ))
    }
    return out
}
