// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf.kotlin

import io.quad4.lxmf.Identity
import io.quad4.lxmf.Lxmf
import io.quad4.lxmf.LxmfException
import io.quad4.lxmf.Message

/** Kotlin facade over the Java liblxmf JNA bindings (ABI 1.0). */
object LxmfKt {
    const val API_VERSION: String = Lxmf.API_VERSION
    const val HASH_LEN: Int = Lxmf.HASH_LEN

    fun version(): String = Lxmf.version()

    fun lastError(): String = Lxmf.lastError()

    fun stampCostFromAppData(appData: ByteArray?): Long = Lxmf.stampCostFromAppData(appData)

    fun pnStampCostFromAppData(appData: ByteArray?): Long = Lxmf.pnStampCostFromAppData(appData)

    fun encodeAnnounceAppData(
        displayName: String?,
        stampCost: Long,
        iconName: String? = null,
        fgRgb3: ByteArray? = null,
        bgRgb3: ByteArray? = null,
    ): ByteArray = Lxmf.encodeAnnounceAppData(displayName, stampCost, iconName, fgRgb3, bgRgb3)
}

typealias LxmfIdentity = Identity
typealias LxmfMessage = Message
typealias LxmfError = LxmfException
