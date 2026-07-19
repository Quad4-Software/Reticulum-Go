// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style pageserver over Swift librns bindings.
// Usage: RNSPageserver -c config [-i identity] [-a announce_sec] [-p page_file]

import Foundation
import RNS

private let defaultAnnounceSec = 900
private let defaultPagePath = "/page/index.mu"
private let defaultFilePath = "/file/test.txt"
private let defaultPageFile = "pages/index.mu"
private let defaultFileFile = "files/test.txt"
private let defaultIdentityPath = "identity"
private let reqDataCap = 64 * 1024

private let fallbackPage = "> Swift pageserver\n\nlibrns via Reticulum-Go\n\nFallback page (file not found).\n\n"
private let fallbackFile = "Test file from Reticulum-Go node!\n"

@main
struct RNSPageserver {
    static func main() {
        let args = Array(CommandLine.arguments.dropFirst())
        var configPath: String?
        var identityPath = defaultIdentityPath
        var announceSec = defaultAnnounceSec
        var pageFile = defaultPageFile
        var fileFile = defaultFileFile
        var requestPath = defaultPagePath
        var i = 0
        while i < args.count {
            let a = args[i]
            if a == "-c" {
                i += 1
                guard i < args.count else { die("missing -c") }
                configPath = args[i]
            } else if a == "-i" {
                i += 1
                guard i < args.count else { die("missing -i") }
                identityPath = args[i]
            } else if a == "-a" {
                i += 1
                guard i < args.count, let v = Int(args[i]), v >= 0 else { die("bad -a") }
                announceSec = v
            } else if a == "-p" {
                i += 1
                guard i < args.count else { die("missing -p") }
                pageFile = args[i]
            } else if a == "-f" {
                i += 1
                guard i < args.count else { die("missing -f") }
                fileFile = args[i]
            } else if a == "-P" {
                i += 1
                guard i < args.count else { die("missing -P") }
                requestPath = args[i]
            } else {
                die("unexpected argument: \(a)")
            }
            i += 1
        }
        guard let configPath else {
            die("usage: RNSPageserver -c config [options]")
        }

        do {
            try run(
                configPath: configPath,
                identityPath: identityPath,
                announceSec: announceSec,
                pageFile: pageFile,
                fileFile: fileFile,
                requestPath: requestPath
            )
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

    static func loadBytes(path: String, fallback: String) -> Data {
        if let data = try? Data(contentsOf: URL(fileURLWithPath: path)) {
            return data
        }
        fputs("warning: could not read \(path), using built-in content\n", stderr)
        return Data(fallback.utf8)
    }

    static func loadOrCreateIdentity(path: String) throws -> Identity {
        if let identity = try? Identity.load(path: path) {
            fputs("loaded identity from \(path)\n", stderr)
            return identity
        }
        let identity = try Identity.generate()
        try identity.save(path: path)
        fputs("created and saved identity to \(path)\n", stderr)
        return identity
    }

    static func run(
        configPath: String,
        identityPath: String,
        announceSec: Int,
        pageFile: String,
        fileFile: String,
        requestPath: String
    ) throws {
        let ver = version()
        guard ver == API_VERSION else {
            die("librns version mismatch: got \(ver)")
        }

        let pageBody = loadBytes(path: pageFile, fallback: fallbackPage)
        let fileBody = loadBytes(path: fileFile, fallback: fallbackFile)

        let node = try Node.create(configPath: configPath)
        defer { node.close() }
        let identity = try loadOrCreateIdentity(path: identityPath)
        defer { identity.close() }
        try node.setIdentity(identity)
        try node.start()

        let dest = try Destination.create(
            node: node,
            identity: nil,
            appName: "nomadnetwork",
            aspects: ["node"],
            acceptsLinks: true
        )
        defer { dest.close() }
        try dest.registerRequestHandler(path: requestPath)
        try dest.registerRequestHandler(path: defaultFilePath)

        let destHex = try hashToHex(try dest.hash())
        print("DEST_HASH=\(destHex)")
        print("REQUEST_PATH=\(requestPath)")
        print("FILE_PATH=\(defaultFilePath)")
        fputs("librns \(ver) pageserver listening as nomadnetwork.node\n", stderr)
        fputs("announce name=librns-swift-pageserver interval=\(announceSec)s\n", stderr)

        let appData = Data("librns-swift-pageserver".utf8)
        do {
            try dest.announce(appData: appData)
            fputs("announce sent\n", stderr)
        } catch {
            printLastError("destination.announce failed")
        }

        var lastAnnounce = Date()
        while true {
            if announceSec > 0 && Date().timeIntervalSince(lastAnnounce) >= TimeInterval(announceSec) {
                try? dest.announce(appData: appData)
                fputs("announce refreshed\n", stderr)
                lastAnnounce = Date()
            }

            do {
                let ev = try Event.poll(node: node, timeoutMs: 200, appDataCapacity: reqDataCap)
                switch ev.kind {
                case .linkEstablished:
                    fputs("inbound link established\n", stderr)
                case .linkClosed:
                    fputs("link closed\n", stderr)
                case .requestIncoming:
                    let path = ev.path
                    fputs("request incoming path=\(path)\n", stderr)
                    let reqId = ev.requestId
                    if path == requestPath {
                        do {
                            try requestRespond(node: node, requestId: reqId, data: pageBody)
                            fputs("served \(requestPath) (\(pageBody.count) bytes)\n", stderr)
                        } catch {
                            printLastError("request_respond failed")
                        }
                    } else if path == defaultFilePath {
                        do {
                            try requestRespondFile(node: node, requestId: reqId, filename: "test.txt", data: fileBody)
                            fputs("served \(defaultFilePath) (\(fileBody.count) bytes)\n", stderr)
                        } catch {
                            printLastError("request_respond_file failed")
                        }
                    } else {
                        try? requestRespond(node: node, requestId: reqId, data: Data("page not found\n".utf8))
                    }
                default:
                    break
                }
            } catch let err as RNSError where err == .timeout {
                continue
            }
        }
    }
}
