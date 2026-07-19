// swift-tools-version: 5.9
// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import PackageDescription

let package = Package(
    name: "RNS",
    platforms: [
        .macOS(.v13),
    ],
    products: [
        .library(name: "RNS", targets: ["RNS"]),
        .executable(name: "RNSSmoke", targets: ["RNSSmoke"]),
        .executable(name: "RNSPageFetch", targets: ["RNSPageFetch"]),
        .executable(name: "RNSPageserver", targets: ["RNSPageserver"]),
    ],
    targets: [
        .systemLibrary(
            name: "CRNS",
            path: "Sources/CRNS"
        ),
        .target(
            name: "RNS",
            dependencies: ["CRNS"],
            path: "Sources/RNS"
        ),
        .executableTarget(
            name: "RNSSmoke",
            dependencies: ["RNS"],
            path: "examples/smoke",
            exclude: ["Makefile"]
        ),
        .executableTarget(
            name: "RNSPageFetch",
            dependencies: ["RNS"],
            path: "examples/page-fetch",
            exclude: ["Makefile", "config.example"]
        ),
        .executableTarget(
            name: "RNSPageserver",
            dependencies: ["RNS"],
            path: "examples/pageserver",
            exclude: ["Makefile", "config.example", "pages", "files"]
        ),
        .testTarget(
            name: "RNSTests",
            dependencies: ["RNS"],
            path: "Tests/RNSTests"
        ),
    ]
)
