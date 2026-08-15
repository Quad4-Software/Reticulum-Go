// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import CRNS

public enum EventKind: Int32 {
    case none = 0
    case announce = 1
    case linkEstablished = 2
    case linkFailed = 3
    case linkData = 4
    case linkClosed = 5
    case requestIncoming = 6
    case requestResponse = 7
    case requestFailed = 8
    case resourceStarted = 9
    case resourceConcluded = 10
    case destinationData = 11
}

public struct Event {
    public let kind: EventKind
    public let hops: UInt8
    public let linkId: Data
    public let destinationHash: Data
    public let identityHash: Data
    public let requestId: Data
    public let path: String
    public let errorMessage: String
    public let appData: Data
    public let pathTruncated: Bool
    public let errorMessageTruncated: Bool
    public let appDataTruncated: Bool

    public static func poll(node: Node, timeoutMs: Int32, appDataCapacity: Int = 0) throws -> Event {
        var event = rns_event()
        var storage = [UInt8](repeating: 0, count: max(appDataCapacity, 0))
        let code = storage.withUnsafeMutableBufferPointer { ptr -> Int32 in
            if appDataCapacity > 0 {
                event.app_data = ptr.baseAddress
                event.app_data_cap = appDataCapacity
            }
            return rns_event_poll(node.rawHandle, &event, timeoutMs)
        }
        try check(code)

        let appData: Data
        if event.app_data_len > 0 && appDataCapacity > 0 {
            appData = Data(storage.prefix(event.app_data_len))
        } else {
            appData = Data()
        }

        return Event(
            kind: EventKind(rawValue: event.kind) ?? .none,
            hops: event.hops,
            linkId: withUnsafeBytes(of: event.link_id) { Data($0.prefix(event.link_id_len)) },
            destinationHash: withUnsafeBytes(of: event.destination_hash) { Data($0.prefix(event.destination_hash_len)) },
            identityHash: withUnsafeBytes(of: event.identity_hash) { Data($0.prefix(event.identity_hash_len)) },
            requestId: withUnsafeBytes(of: event.request_id) { Data($0.prefix(event.request_id_len)) },
            path: withUnsafeBytes(of: event.path) { String(cString: $0.bindMemory(to: CChar.self).baseAddress!) },
            errorMessage: withUnsafeBytes(of: event.error_message) { String(cString: $0.bindMemory(to: CChar.self).baseAddress!) },
            appData: appData,
            pathTruncated: event.path_truncated != 0,
            errorMessageTruncated: event.error_message_truncated != 0,
            appDataTruncated: event.app_data_truncated != 0
        )
    }
}
