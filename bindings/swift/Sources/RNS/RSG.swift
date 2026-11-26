// SPDX-License-Identifier: LicenseRef-Reticulum
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

private func validPath(_ path: String) -> Bool {
    !path.isEmpty && !path.contains("\0")
}

public func rsgSignFile(identity: Identity, path: String) throws -> Data {
    guard validPath(path) else { throw RNSError.invalidArgument }
    var needed: Int = 0
    let probe: Int32 = path.withCString { rns_rsg_sign_file(identity.rawHandle, $0, nil, 0, &needed) }
    if probe != RNS_OK && probe != RNS_ERR_TRUNCATED {
        try check(probe)
    }
    var out = [UInt8](repeating: 0, count: needed)
    var written: Int = 0
    try path.withCString { cPath in
        try out.withUnsafeMutableBufferPointer { buf in
            try check(rns_rsg_sign_file(identity.rawHandle, cPath, buf.baseAddress, needed, &written))
        }
    }
    return Data(out.prefix(written))
}

public func rsgVerifyFile(rsg: Data, path: String, requiredSignerHash: Data = Data()) throws {
    guard !rsg.isEmpty, validPath(path) else { throw RNSError.invalidArgument }
    try path.withCString { cPath in
        try rsg.withUnsafeBytes { rRaw in
            try requiredSignerHash.withUnsafeBytes { sRaw in
                try check(rns_rsg_verify_file(
                    rRaw.bindMemory(to: UInt8.self).baseAddress,
                    rsg.count,
                    cPath,
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
