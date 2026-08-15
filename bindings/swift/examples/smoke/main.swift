// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import Foundation
import RNS

@main
struct RNSSmoke {
    static func main() {
        let ver = version()
        guard ver == API_VERSION else {
            fputs("unexpected version: \(ver)\n", stderr)
            exit(1)
        }

        do {
            let node = try Node.create()
            defer { node.close() }
            try node.start()
            do {
                _ = try Event.poll(node: node, timeoutMs: 10)
                fputs("expected timeout poll on idle node\n", stderr)
                exit(1)
            } catch let err as RNSError where err == .timeout {
                // ok
            }
            try node.stop()
        } catch {
            fputs("\(error)\n", stderr)
            exit(1)
        }

        print("swift-smoke ok")
    }
}
