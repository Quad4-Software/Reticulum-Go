// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public func rsgCreate(identity: Identity, message: Data, embed: Bool = true) throws -> Data {
    var needed: Int = 0
    let probe: Int32 = message.withUnsafeBytes { raw in
        rns_rsg_create(
            identity.rawHandle,
            raw.bindMemory(to: UInt8.self).baseAddress,
            message.count,
            embed ? 1 : 0,
            nil,
            0,
            &needed
        )
    }
    if probe != RNS_OK && probe != RNS_ERR_TRUNCATED {
        try check(probe)
    }
    guard needed > 0 else { throw RNSError.internalError }
    var out = [UInt8](repeating: 0, count: needed)
    var written: Int = 0
    try message.withUnsafeBytes { raw in
        try out.withUnsafeMutableBufferPointer { buf in
            try check(rns_rsg_create(
                identity.rawHandle,
                raw.bindMemory(to: UInt8.self).baseAddress,
                message.count,
                embed ? 1 : 0,
                buf.baseAddress,
                needed,
                &written
            ))
        }
    }
    return Data(out.prefix(written))
}

public func rsgValidate(rsg: Data, message: Data, requiredSignerHash: Data = Data()) throws {
    try rsg.withUnsafeBytes { rRaw in
        try message.withUnsafeBytes { mRaw in
            try requiredSignerHash.withUnsafeBytes { sRaw in
                try check(rns_rsg_validate(
                    rRaw.bindMemory(to: UInt8.self).baseAddress,
                    rsg.count,
                    mRaw.bindMemory(to: UInt8.self).baseAddress,
                    message.count,
                    sRaw.bindMemory(to: UInt8.self).baseAddress,
                    requiredSignerHash.count
                ))
            }
        }
    }
}

public func rsmVerify(rsm: Data, requiredSignerHash: Data = Data()) throws -> Data {
    var needed: Int = 0
    let probe: Int32 = rsm.withUnsafeBytes { rRaw in
        requiredSignerHash.withUnsafeBytes { sRaw in
            rns_rsm_verify(
                rRaw.bindMemory(to: UInt8.self).baseAddress,
                rsm.count,
                sRaw.bindMemory(to: UInt8.self).baseAddress,
                requiredSignerHash.count,
                nil,
                0,
                &needed
            )
        }
    }
    if probe != RNS_OK && probe != RNS_ERR_TRUNCATED {
        try check(probe)
    }
    if needed == 0 {
        return Data()
    }
    var out = [UInt8](repeating: 0, count: needed)
    var written: Int = 0
    try rsm.withUnsafeBytes { rRaw in
        try requiredSignerHash.withUnsafeBytes { sRaw in
            try out.withUnsafeMutableBufferPointer { buf in
                try check(rns_rsm_verify(
                    rRaw.bindMemory(to: UInt8.self).baseAddress,
                    rsm.count,
                    sRaw.bindMemory(to: UInt8.self).baseAddress,
                    requiredSignerHash.count,
                    buf.baseAddress,
                    needed,
                    &written
                ))
            }
        }
    }
    return Data(out.prefix(written))
}
