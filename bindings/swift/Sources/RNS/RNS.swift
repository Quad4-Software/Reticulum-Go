// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public let API_VERSION = "1.5"
public let HASH_LEN = 16

public enum RNSError: Error, Equatable {
    case invalidArgument
    case invalidHandle
    case notFound
    case state
    case io
    case internalError
    case timeout
    case truncated
    case unknown(Int32)

    public var code: Int32 {
        switch self {
        case .invalidArgument: return RNS_ERR_INVALID_ARG
        case .invalidHandle: return RNS_ERR_INVALID_HANDLE
        case .notFound: return RNS_ERR_NOT_FOUND
        case .state: return RNS_ERR_STATE
        case .io: return RNS_ERR_IO
        case .internalError: return RNS_ERR_INTERNAL
        case .timeout: return RNS_ERR_TIMEOUT
        case .truncated: return RNS_ERR_TRUNCATED
        case .unknown(let code): return code
        }
    }

    public static func from(code: Int32) -> RNSError {
        switch code {
        case RNS_ERR_INVALID_ARG: return .invalidArgument
        case RNS_ERR_INVALID_HANDLE: return .invalidHandle
        case RNS_ERR_NOT_FOUND: return .notFound
        case RNS_ERR_STATE: return .state
        case RNS_ERR_IO: return .io
        case RNS_ERR_INTERNAL: return .internalError
        case RNS_ERR_TIMEOUT: return .timeout
        case RNS_ERR_TRUNCATED: return .truncated
        default: return .unknown(code)
        }
    }
}

@discardableResult
public func check(_ code: Int32) throws -> Int32 {
    if code == RNS_OK {
        return code
    }
    throw RNSError.from(code: code)
}

public func version() -> String {
    guard let raw = rns_version() else {
        return ""
    }
    return String(cString: raw)
}

public func lastError() -> String {
    var buf = [CChar](repeating: 0, count: 512)
    var written: Int = 0
    let code = buf.withUnsafeMutableBufferPointer { ptr in
        rns_last_error(ptr.baseAddress, ptr.count, &written)
    }
    if code != RNS_OK || written == 0 {
        return ""
    }
    return String(cString: buf)
}

public func hashToHex(_ data: Data) throws -> String {
    guard data.count == HASH_LEN else {
        throw RNSError.invalidArgument
    }
    return data.map { String(format: "%02x", $0) }.joined()
}

public func hexToHash(_ text: String) throws -> Data {
    guard text.count == 32 else {
        throw RNSError.invalidArgument
    }
    var out = Data(capacity: HASH_LEN)
    var idx = text.startIndex
    for _ in 0..<HASH_LEN {
        let next = text.index(idx, offsetBy: 2)
        guard let byte = UInt8(text[idx..<next], radix: 16) else {
            throw RNSError.invalidArgument
        }
        out.append(byte)
        idx = next
    }
    return out
}

public func hashEq(_ a: Data, _ b: Data) -> Bool {
    a.count == HASH_LEN && b.count == HASH_LEN && a == b
}
