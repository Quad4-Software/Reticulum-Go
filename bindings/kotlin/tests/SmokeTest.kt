// SPDX-License-Identifier: LicenseRef-Reticulum
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

    RnsNode.create().use { node ->
        try {
            RnsKt.nodeReloadConfig(node)
            error("expected reload failure")
        } catch (e: RnsError) {
            check(e.code == RnsError.INVALID_ARG)
        }
        val config = java.nio.file.Files.createTempFile("rns-kotlin-reload", ".conf")
        java.nio.file.Files.write(config, "[reticulum]\nenable_transport = no\nin_memory_storage = yes\n".toByteArray())
        RnsNode.create(config.toString()).use { configured ->
            RnsKt.nodeReloadConfig(configured)
        }
        java.nio.file.Files.deleteIfExists(config)
        RnsKt.nodeRefreshPaths(node, emptyArray())
        try {
            RnsKt.destinationEncrypt(ByteArray(16), byteArrayOf(120))
            error("expected encrypt failure")
        } catch (e: RnsError) {
            check(e.code == RnsError.NOT_FOUND)
        }
        try {
            RnsKt.packetSend(node, ByteArray(16), byteArrayOf(120))
            error("expected packet failure")
        } catch (e: RnsError) {
            check(e.code == RnsError.STATE || e.code == RnsError.NOT_FOUND)
        }
        try {
            RnsKt.linkFromId(node, ByteArray(16))
            error("expected missing link")
        } catch (e: RnsError) {
            check(e.code == RnsError.NOT_FOUND)
        }
        RnsKt.nodeSetEventCallback(node) { }
        RnsKt.nodeSetEventCallback(node, null)
    }

    println("kotlin smoke tests ok")
}
