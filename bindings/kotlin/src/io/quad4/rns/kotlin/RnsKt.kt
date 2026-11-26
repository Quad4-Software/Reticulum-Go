// SPDX-License-Identifier: LicenseRef-Reticulum
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

/** Kotlin facade over the Java librns JNA bindings (ABI 1.6). */
object RnsKt {
    const val API_VERSION: String = Rns.API_VERSION
    const val HASH_LEN: Int = Rns.HASH_LEN

    fun version(): String = Rns.version()

    fun lastError(): String = Rns.lastError()

    fun hashToHex(data: ByteArray): String = Rns.hashToHex(data)

    fun hexToHash(text: String): ByteArray = Rns.hexToHash(text)

    fun hashEq(a: ByteArray, b: ByteArray): Boolean = Rns.hashEq(a, b)

    fun nodeReloadConfig(node: Node) = node.reloadConfig()

    fun nodeRefreshPaths(node: Node, destHashes: Array<ByteArray>) = node.refreshPaths(destHashes)

    fun packetSend(node: Node, destHash: ByteArray, plaintext: ByteArray) =
        Rns.packetSend(node, destHash, plaintext)

    fun destinationEncrypt(destHash: ByteArray, plaintext: ByteArray): ByteArray =
        Rns.destinationEncrypt(destHash, plaintext)

    fun nodeSetEventCallback(node: Node, handler: ((Event) -> Unit)?) {
        if (handler == null) {
            node.setEventCallback(null)
        } else {
            node.setEventCallback { event -> handler(event) }
        }
    }

    fun destinationSetProofStrategy(destination: Destination, strategy: Int) =
        destination.setProofStrategy(strategy)

    fun linkFromId(node: Node, linkId: ByteArray): Link = Link.fromId(node, linkId)

    fun rsgSignFile(identity: Identity, path: String): ByteArray = Rsg.signFile(identity, path)

    fun rsgVerifyFile(rsg: ByteArray, path: String, requiredSignerHash: ByteArray = ByteArray(0)) =
        Rsg.verifyFile(rsg, path, requiredSignerHash)
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
