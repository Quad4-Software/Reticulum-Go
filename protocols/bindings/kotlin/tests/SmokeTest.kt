// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf.kotlin

fun main() {
    check(LxmfKt.version() == LxmfKt.API_VERSION) { "version mismatch: ${LxmfKt.version()}" }

    LxmfIdentity.generate().use { identity ->
        val hash = identity.hashBytes()
        check(hash.size == LxmfKt.HASH_LEN) { "hash length" }
        check(identity.publicKey().isNotEmpty()) { "public key empty" }
        check(identity.deliveryHash().size == LxmfKt.HASH_LEN) { "delivery hash length" }

        LxmfMessage.create(hash, hash, "hi", "hello lxmf").use { msg ->
            val packed = msg.pack(identity)
            check(packed.isNotEmpty()) { "pack empty" }
            val inner = msg.encryptedPayload()
            check(inner.size == packed.size - LxmfKt.HASH_LEN) { "encrypted payload length" }
            LxmfMessage.unpack(packed).use { got ->
                check(got.content() == "hello lxmf") { "content mismatch: ${got.content()}" }
                check(got.title() == "hi") { "title mismatch: ${got.title()}" }
            }
        }
    }

    println("kotlin-lxmf smoke tests ok")
}
