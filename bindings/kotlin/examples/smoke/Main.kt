// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import io.quad4.rns.kotlin.RnsError
import io.quad4.rns.kotlin.RnsEvent
import io.quad4.rns.kotlin.RnsKt
import io.quad4.rns.kotlin.RnsNode

fun main() {
    check(RnsKt.version() == RnsKt.API_VERSION) { "unexpected version: ${RnsKt.version()}" }
    RnsNode.create().use { node ->
        node.start()
        try {
            RnsEvent.poll(node, 10, 0)
            System.err.println("expected timeout poll on idle node")
            kotlin.system.exitProcess(1)
        } catch (e: RnsError) {
            if (e.code != RnsError.TIMEOUT) {
                System.err.println("expected timeout poll on idle node")
                kotlin.system.exitProcess(1)
            }
        }
        node.stop()
    }
    println("kotlin-smoke ok")
}
