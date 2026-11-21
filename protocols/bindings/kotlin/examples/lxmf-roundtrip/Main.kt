// SPDX-License-Identifier: LicenseRef-Reticulum

import io.quad4.lxmf.kotlin.LxmfIdentity
import io.quad4.lxmf.kotlin.LxmfKt
import io.quad4.lxmf.kotlin.LxmfMessage

fun main() {
    check(LxmfKt.version() == LxmfKt.API_VERSION) { "unexpected version: ${LxmfKt.version()}" }
    LxmfIdentity.generate().use { identity ->
        val hash = identity.hashBytes()
        LxmfMessage.create(hash, hash, "hi", "hello lxmf").use { msg ->
            val packed = msg.pack(identity)
            val inner = msg.encryptedPayload()
            check(inner.size == packed.size - LxmfKt.HASH_LEN) { "encrypted payload length" }
            LxmfMessage.unpack(packed).use { got ->
                check(got.content() == "hello lxmf") { "content mismatch" }
            }
        }
    }
    println("kotlin-lxmf-roundtrip ok")
}
