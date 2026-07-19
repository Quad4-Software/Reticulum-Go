// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style page fetch over Kotlin librns bindings.
// Usage: kotlin MainKt -c config [-t timeout_sec] <dest_hash>:<page_path>

import io.quad4.rns.kotlin.RnsError
import io.quad4.rns.kotlin.RnsEvent
import io.quad4.rns.kotlin.RnsIdentity
import io.quad4.rns.kotlin.RnsKt
import io.quad4.rns.kotlin.RnsLink
import io.quad4.rns.kotlin.RnsNode
import io.quad4.rns.kotlin.RnsPath

private const val PAGE_BUF_CAP = 512 * 1024
private const val DEFAULT_TIMEOUT_SEC = 60
private const val PATH_RETRY_MS = 2000L

fun main(args: Array<String>) {
    var configPath: String? = null
    var timeoutSec = DEFAULT_TIMEOUT_SEC
    var target: String? = null
    var i = 0
    while (i < args.size) {
        when (args[i]) {
            "-c" -> {
                i += 1
                configPath = args.getOrNull(i) ?: die("missing -c")
            }
            "-t" -> {
                i += 1
                timeoutSec = args.getOrNull(i)?.toIntOrNull()?.takeIf { it > 0 } ?: die("bad -t")
            }
            else -> {
                if (target == null) target = args[i] else die("unexpected argument: ${args[i]}")
            }
        }
        i += 1
    }
    if (configPath == null || target == null) {
        die("usage: MainKt -c config [-t timeout] <dest_hash>:<page_path>")
    }
    check(RnsKt.version() == RnsKt.API_VERSION) { "librns version mismatch: ${RnsKt.version()}" }

    val colon = target!!.indexOf(':')
    if (colon <= 0 || colon == target!!.lastIndex) die("target must be <32-hex-dest>:<page_path>")
    val destHash = RnsKt.hexToHash(target!!.substring(0, colon))
    val pagePath = target!!.substring(colon + 1)
    val destHex = RnsKt.hashToHex(destHash)

    RnsNode.create(configPath).use { node ->
        RnsIdentity.generate().use { identity ->
            node.setIdentity(identity)
            node.start()
            println("librns ${RnsKt.version()} fetching $pagePath from $destHex")

            val deadline = System.currentTimeMillis() + timeoutSec * 1000L
            var lastPathReq = 0L
            var needPathReq = true
            var sawAnnounce = false
            var link: RnsLink? = null

            while (System.currentTimeMillis() < deadline && link == null) {
                val now = System.currentTimeMillis()
                if (needPathReq || now - lastPathReq >= PATH_RETRY_MS) {
                    runCatching { RnsPath.request(node, destHash) }.onFailure {
                        printLastError("path_request failed")
                    }
                    lastPathReq = now
                    needPathReq = false
                    if (RnsPath.known(node, destHash)) {
                        System.err.println("path known, waiting for destination identity announce")
                    } else {
                        System.err.println("requesting path to $destHex")
                    }
                }
                try {
                    val ev = RnsEvent.poll(node, 200, PAGE_BUF_CAP)
                    if (ev.kind() == RnsEvent.ANNOUNCE && RnsKt.hashEq(ev.destinationHash(), destHash)) {
                        sawAnnounce = true
                        System.err.println("announce for target (hops=${ev.hops()})")
                        link = runCatching { RnsLink.open(node, destHash) }.getOrElse {
                            printLastError("Link.open after announce")
                            null
                        }
                    } else if (ev.kind() == RnsEvent.LINK_FAILED) {
                        System.err.println("link failed while opening: ${ev.errorMessage()}")
                    }
                } catch (e: RnsError) {
                    if (e.code == RnsError.TIMEOUT) {
                        if (sawAnnounce || RnsPath.known(node, destHash)) {
                            link = runCatching { RnsLink.open(node, destHash) }.getOrNull()
                        }
                    } else {
                        printLastError("Event.poll failed")
                        kotlin.system.exitProcess(1)
                    }
                }
            }

            val opened = link ?: die("timed out before link open")
            opened.use {
                var established = false
                while (System.currentTimeMillis() < deadline && !established) {
                    try {
                        val ev = RnsEvent.poll(node, 500, PAGE_BUF_CAP)
                        when (ev.kind()) {
                            RnsEvent.LINK_ESTABLISHED -> {
                                established = true
                                System.err.println("link established")
                            }
                            RnsEvent.LINK_FAILED -> die("link establishment failed: ${ev.errorMessage()}")
                            RnsEvent.LINK_CLOSED -> die("link closed before establish")
                        }
                    } catch (e: RnsError) {
                        if (e.code != RnsError.TIMEOUT) {
                            printLastError("Event.poll failed")
                            kotlin.system.exitProcess(1)
                        }
                    }
                }
                if (!established) die("timed out waiting for link establishment")

                val remainingMs = (deadline - System.currentTimeMillis()).coerceAtLeast(1000L).toInt()
                opened.request(node, pagePath, ByteArray(0), remainingMs)
                System.err.println("request sent for $pagePath")

                while (System.currentTimeMillis() < deadline) {
                    try {
                        val ev = RnsEvent.poll(node, 500, PAGE_BUF_CAP)
                        when (ev.kind()) {
                            RnsEvent.REQUEST_RESPONSE -> {
                                val data = ev.appData()
                                println("\n=== Page Content (${data.size} bytes) ===")
                                print(data.toString(Charsets.UTF_8))
                                if (data.isEmpty() || data.last() != '\n'.code.toByte()) println()
                                if (ev.appDataTruncated()) System.err.println("warning: response truncated")
                                println("=== End of Page ===")
                                return
                            }
                            RnsEvent.REQUEST_FAILED -> die("request failed: ${ev.errorMessage()}")
                            RnsEvent.LINK_CLOSED -> die("link closed before response")
                        }
                    } catch (e: RnsError) {
                        if (e.code != RnsError.TIMEOUT) {
                            printLastError("Event.poll failed")
                            kotlin.system.exitProcess(1)
                        }
                    }
                }
                die("timed out waiting for page response")
            }
        }
    }
}

private fun die(msg: String): Nothing {
    System.err.println(msg)
    kotlin.system.exitProcess(1)
}

private fun printLastError(what: String) {
    val msg = RnsKt.lastError()
    if (msg.isEmpty()) System.err.println(what) else System.err.println("$what: $msg")
}
