// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.kotlin

fun main() {
    check(RnsKt.version() == RnsKt.API_VERSION) { "version mismatch: ${RnsKt.version()}" }

    RnsNode.create().use { node ->
        node.start()
        try {
            RnsEvent.poll(node, 10, 0)
            error("expected timeout")
        } catch (e: RnsError) {
            check(e.code == RnsError.TIMEOUT) { "expected timeout code" }
        }
        node.stop()
    }

    RnsIdentity.generate().use { identity ->
        check(identity.hashHex().length == 32)
        check(identity.hashBytes().size == 16)
        val msg = "hello-rns".toByteArray()
        val sig = identity.sign(msg)
        identity.verify(msg, sig)
        val pub = identity.publicKey()
        RnsIdentity.fromPublicKey(pub).use { onlyPub ->
            onlyPub.verify(msg, sig)
        }
    }

    println("kotlin smoke tests ok")
}
