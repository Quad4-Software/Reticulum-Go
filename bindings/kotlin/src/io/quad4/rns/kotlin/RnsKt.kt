// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.kotlin

import io.quad4.rns.Destination
import io.quad4.rns.Event
import io.quad4.rns.Identity
import io.quad4.rns.Interfaces
import io.quad4.rns.Link
import io.quad4.rns.Node
import io.quad4.rns.Path
import io.quad4.rns.Rns
import io.quad4.rns.RnsException
import io.quad4.rns.Rsg

/** Kotlin facade over the Java librns JNA bindings (ABI 1.5). */
object RnsKt {
    const val API_VERSION: String = Rns.API_VERSION
    const val HASH_LEN: Int = Rns.HASH_LEN

    fun version(): String = Rns.version()

    fun lastError(): String = Rns.lastError()

    fun hashToHex(data: ByteArray): String = Rns.hashToHex(data)

    fun hexToHash(text: String): ByteArray = Rns.hexToHash(text)

    fun hashEq(a: ByteArray, b: ByteArray): Boolean = Rns.hashEq(a, b)
}

typealias RnsNode = Node
typealias RnsIdentity = Identity
typealias RnsDestination = Destination
typealias RnsLink = Link
typealias RnsEvent = Event
typealias RnsPath = Path
typealias RnsInterfaces = Interfaces
typealias RnsRsg = Rsg
typealias RnsError = RnsException
