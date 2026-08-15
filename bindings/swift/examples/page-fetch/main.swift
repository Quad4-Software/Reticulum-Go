// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style page fetch over Swift librns bindings.
// Usage: RNSPageFetch -c config [-t timeout_sec] <dest_hash>:<page_path>

import Foundation
import RNS

private let pageBufCap = 512 * 1024
private let defaultTimeoutSec = 60
private let pathRetrySec = 2.0

@main
struct RNSPageFetch {
    static func main() {
        let args = Array(CommandLine.arguments.dropFirst())
        var configPath: String?
        var timeoutSec = defaultTimeoutSec
        var target: String?
        var i = 0
        while i < args.count {
            let a = args[i]
            if a == "-c" {
                i += 1
                guard i < args.count else { die("missing -c value") }
                configPath = args[i]
            } else if a == "-t" {
                i += 1
                guard i < args.count, let v = Int(args[i]), v > 0 else { die("bad -t") }
                timeoutSec = v
            } else if target == nil {
                target = a
            } else {
                die("unexpected argument: \(a)")
            }
            i += 1
        }
        guard let configPath, let target else {
            die("usage: RNSPageFetch -c config [-t timeout] <dest_hash>:<page_path>")
        }

        do {
            try run(configPath: configPath, target: target, timeoutSec: timeoutSec)
        } catch {
            fputs("\(error)\n", stderr)
            exit(1)
        }
    }

    static func die(_ msg: String) -> Never {
        fputs(msg + "\n", stderr)
        exit(1)
    }

    static func printLastError(_ what: String) {
        let msg = lastError()
        if msg.isEmpty {
            fputs(what + "\n", stderr)
        } else {
            fputs("\(what): \(msg)\n", stderr)
        }
    }

    static func parseTarget(_ target: String) throws -> (Data, String) {
        guard let idx = target.firstIndex(of: ":") else { throw RNSError.invalidArgument }
        let hexPart = String(target[..<idx])
        let pagePath = String(target[target.index(after: idx)...])
        guard !hexPart.isEmpty, !pagePath.isEmpty else { throw RNSError.invalidArgument }
        return (try hexToHash(hexPart), pagePath)
    }

    static func run(configPath: String, target: String, timeoutSec: Int) throws {
        let ver = version()
        guard ver == API_VERSION else {
            die("librns version mismatch: got \(ver)")
        }
        let (destHash, pagePath) = try parseTarget(target)
        let destHex = try hashToHex(destHash)

        let node = try Node.create(configPath: configPath)
        defer { node.close() }
        let identity = try Identity.generate()
        defer { identity.close() }
        try node.setIdentity(identity)
        try node.start()

        print("librns \(ver) fetching \(pagePath) from \(destHex)")

        let deadline = Date().addingTimeInterval(TimeInterval(timeoutSec))
        var lastPathReq = Date.distantPast
        var needPathReq = true
        var sawAnnounce = false
        var link: Link?

        while Date() < deadline && link == nil {
            let now = Date()
            if needPathReq || now.timeIntervalSince(lastPathReq) >= pathRetrySec {
                do {
                    try pathRequest(node: node, destHash: destHash)
                } catch {
                    printLastError("path_request failed")
                }
                lastPathReq = now
                needPathReq = false
                if pathKnown(node: node, destHash: destHash) {
                    fputs("path known, waiting for destination identity announce\n", stderr)
                } else {
                    fputs("requesting path to \(destHex)\n", stderr)
                }
            }

            do {
                let ev = try Event.poll(node: node, timeoutMs: 200, appDataCapacity: pageBufCap)
                if ev.kind == .announce && hashEq(ev.destinationHash, destHash) {
                    sawAnnounce = true
                    fputs("announce for target (hops=\(ev.hops))\n", stderr)
                    do {
                        link = try Link.open(node: node, destHash: destHash)
                    } catch {
                        printLastError("Link.open after announce")
                    }
                } else if ev.kind == .linkFailed {
                    fputs("link failed while opening: \(ev.errorMessage)\n", stderr)
                }
            } catch let err as RNSError where err == .timeout {
                if sawAnnounce || pathKnown(node: node, destHash: destHash) {
                    link = try? Link.open(node: node, destHash: destHash)
                }
            }
        }

        guard let link else {
            die("timed out before link open")
        }
        defer { link.close() }

        var established = false
        while Date() < deadline && !established {
            do {
                let ev = try Event.poll(node: node, timeoutMs: 500, appDataCapacity: pageBufCap)
                if ev.kind == .linkEstablished {
                    established = true
                    fputs("link established\n", stderr)
                } else if ev.kind == .linkFailed {
                    die("link establishment failed: \(ev.errorMessage)")
                } else if ev.kind == .linkClosed {
                    die("link closed before establish")
                }
            } catch let err as RNSError where err == .timeout {
                continue
            }
        }
        guard established else {
            die("timed out waiting for link establishment")
        }

        let remainingMs = Int32(max(Date().distance(to: deadline) * 1000, 1000))
        _ = try link.request(node: node, path: pagePath, data: Data(), timeoutMs: remainingMs)
        fputs("request sent for \(pagePath)\n", stderr)

        while Date() < deadline {
            do {
                let ev = try Event.poll(node: node, timeoutMs: 500, appDataCapacity: pageBufCap)
                if ev.kind == .requestResponse {
                    let data = ev.appData
                    print("\n=== Page Content (\(data.count) bytes) ===")
                    if let text = String(data: data, encoding: .utf8) {
                        print(text, terminator: text.hasSuffix("\n") ? "" : "\n")
                    } else {
                        FileHandle.standardOutput.write(data)
                        print()
                    }
                    if ev.appDataTruncated {
                        fputs("warning: response truncated\n", stderr)
                    }
                    print("=== End of Page ===")
                    return
                } else if ev.kind == .requestFailed {
                    die("request failed: \(ev.errorMessage)")
                } else if ev.kind == .linkClosed {
                    die("link closed before response")
                }
            } catch let err as RNSError where err == .timeout {
                continue
            }
        }
        die("timed out waiting for page response")
    }
}
