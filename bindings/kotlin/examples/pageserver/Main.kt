// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style pageserver over Kotlin librns bindings.
// Usage: MainKt -c config [-i identity] [-a announce_sec] [-p page_file]

import io.quad4.rns.kotlin.RnsDestination
import io.quad4.rns.kotlin.RnsError
import io.quad4.rns.kotlin.RnsEvent
import io.quad4.rns.kotlin.RnsIdentity
import io.quad4.rns.kotlin.RnsKt
import io.quad4.rns.kotlin.RnsLink
import io.quad4.rns.kotlin.RnsNode
import java.nio.file.Files
import java.nio.file.Paths

private const val DEFAULT_ANNOUNCE_SEC = 900
private const val DEFAULT_PAGE_PATH = "/page/index.mu"
private const val DEFAULT_FILE_PATH = "/file/test.txt"
private const val DEFAULT_PAGE_FILE = "pages/index.mu"
private const val DEFAULT_FILE_FILE = "files/test.txt"
private const val DEFAULT_IDENTITY_PATH = "identity"
private const val REQ_DATA_CAP = 64 * 1024
private const val FALLBACK_PAGE =
    "> Kotlin pageserver\n\nlibrns via Reticulum-Go\n\nFallback page (file not found).\n\n"
private const val FALLBACK_FILE = "Test file from Reticulum-Go node!\n"

fun main(args: Array<String>) {
    var configPath: String? = null
    var identityPath = DEFAULT_IDENTITY_PATH
    var announceSec = DEFAULT_ANNOUNCE_SEC
    var pageFile = DEFAULT_PAGE_FILE
    var fileFile = DEFAULT_FILE_FILE
    var requestPath = DEFAULT_PAGE_PATH
    var i = 0
    while (i < args.size) {
        when (args[i]) {
            "-c" -> {
                i += 1
                configPath = args.getOrNull(i) ?: die("missing -c")
            }
            "-i" -> {
                i += 1
                identityPath = args.getOrNull(i) ?: die("missing -i")
            }
            "-a" -> {
                i += 1
                announceSec = args.getOrNull(i)?.toIntOrNull()?.takeIf { it >= 0 } ?: die("bad -a")
            }
            "-p" -> {
                i += 1
                pageFile = args.getOrNull(i) ?: die("missing -p")
            }
            "-f" -> {
                i += 1
                fileFile = args.getOrNull(i) ?: die("missing -f")
            }
            "-P" -> {
                i += 1
                requestPath = args.getOrNull(i) ?: die("missing -P")
            }
            else -> die("unexpected argument: ${args[i]}")
        }
        i += 1
    }
    if (configPath == null) die("usage: MainKt -c config [options]")
    check(RnsKt.version() == RnsKt.API_VERSION) { "librns version mismatch: ${RnsKt.version()}" }

    val pageBody = loadBytes(pageFile, FALLBACK_PAGE)
    val fileBody = loadBytes(fileFile, FALLBACK_FILE)

    RnsNode.create(configPath).use { node ->
        loadOrCreateIdentity(identityPath).use { identity ->
            node.setIdentity(identity)
            node.start()
            RnsDestination.create(node, null, "nomadnetwork", listOf("node"), true).use { dest ->
                dest.registerRequestHandler(requestPath)
                dest.registerRequestHandler(DEFAULT_FILE_PATH)

                val destHex = RnsKt.hashToHex(dest.hash())
                println("DEST_HASH=$destHex")
                println("REQUEST_PATH=$requestPath")
                println("FILE_PATH=$DEFAULT_FILE_PATH")
                System.err.println("librns ${RnsKt.version()} pageserver listening as nomadnetwork.node")

                val appData = "librns-kotlin-pageserver".toByteArray()
                runCatching { dest.announce(appData) }
                    .onSuccess { System.err.println("announce sent") }
                    .onFailure { printLastError("destination.announce failed") }

                var lastAnnounce = System.currentTimeMillis()
                while (true) {
                    if (announceSec > 0 && System.currentTimeMillis() - lastAnnounce >= announceSec * 1000L) {
                        runCatching { dest.announce(appData) }
                        System.err.println("announce refreshed")
                        lastAnnounce = System.currentTimeMillis()
                    }
                    try {
                        val ev = RnsEvent.poll(node, 200, REQ_DATA_CAP)
                        when (ev.kind()) {
                            RnsEvent.LINK_ESTABLISHED -> System.err.println("inbound link established")
                            RnsEvent.LINK_CLOSED -> System.err.println("link closed")
                            RnsEvent.REQUEST_INCOMING -> {
                                val path = ev.path()
                                System.err.println("request incoming path=$path")
                                val reqId = ev.requestId()
                                when (path) {
                                    requestPath -> {
                                        runCatching { RnsLink.requestRespond(node, reqId, pageBody) }
                                            .onSuccess {
                                                System.err.println(
                                                    "served $requestPath (${pageBody.size} bytes)"
                                                )
                                            }
                                            .onFailure { printLastError("request_respond failed") }
                                    }
                                    DEFAULT_FILE_PATH -> {
                                        runCatching {
                                            RnsLink.requestRespondFile(node, reqId, "test.txt", fileBody)
                                        }.onSuccess {
                                            System.err.println(
                                                "served $DEFAULT_FILE_PATH (${fileBody.size} bytes)"
                                            )
                                        }.onFailure { printLastError("request_respond_file failed") }
                                    }
                                    else -> {
                                        runCatching {
                                            RnsLink.requestRespond(node, reqId, "page not found\n".toByteArray())
                                        }
                                    }
                                }
                            }
                        }
                    } catch (e: RnsError) {
                        if (e.code != RnsError.TIMEOUT) {
                            printLastError("Event.poll failed")
                            kotlin.system.exitProcess(1)
                        }
                    }
                }
            }
        }
    }
}

private fun loadBytes(path: String, fallback: String): ByteArray =
    try {
        Files.readAllBytes(Paths.get(path))
    } catch (_: Exception) {
        System.err.println("warning: could not read $path, using built-in content")
        fallback.toByteArray()
    }

private fun loadOrCreateIdentity(path: String): RnsIdentity =
    try {
        RnsIdentity.load(path).also { System.err.println("loaded identity from $path") }
    } catch (_: RnsError) {
        RnsIdentity.generate().also {
            it.save(path)
            System.err.println("created and saved identity to $path")
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
