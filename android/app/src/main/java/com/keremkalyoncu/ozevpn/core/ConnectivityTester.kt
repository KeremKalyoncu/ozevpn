package com.keremkalyoncu.ozevpn.core

import com.keremkalyoncu.ozevpn.data.ProbeItem
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.withContext
import java.io.BufferedInputStream
import java.io.ByteArrayOutputStream
import java.net.HttpURLConnection
import java.net.InetSocketAddress
import java.net.Socket
import java.net.URL
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory

data class TraceResult(val colo: String, val country: String, val warpActive: Boolean, val rttMs: Int)

internal data class SimpleHttpResponse(val statusCode: Int, val body: String, val latencyMs: Long)

/**
 * Tests live reachability of Discord, Roblox, Google, and Cloudflare Trace.
 * When the VPN tunnel is active, sends explicit HTTP CONNECT host:443 straight into local
 * sing-box (127.0.0.1:10808) + TLS with strict hostname verification, guaranteeing ZERO local
 * ISP DNS resolution even on carriers that censor Discord/Roblox DNS.
 */
object ConnectivityTester {
    private data class Target(val name: String, val host: String, val path: String)

    private val targets = listOf(
        Target("Discord", "discord.com", "/api/v10/gateway"),
        Target("Roblox", "www.roblox.com", "/favicon.ico"),
        Target("Google", "connectivitycheck.gstatic.com", "/generate_204"),
        Target("Cloudflare", "1.1.1.1", "/cdn-cgi/trace"),
    )

    suspend fun runAll(viaTunnel: Boolean): Pair<List<ProbeItem>, TraceResult?> = withContext(Dispatchers.IO) {
        val items = coroutineScope {
            targets.map { t ->
                async(Dispatchers.IO) { probeTarget(t, viaTunnel) }
            }.awaitAll()
        }
        val trace = fetchTrace(viaTunnel)
        items to trace
    }

    suspend fun fetchTrace(viaTunnel: Boolean): TraceResult? = withContext(Dispatchers.IO) {
        val resp = if (viaTunnel) {
            httpsGetViaConnectProxy(SingBoxConfigBuilder.LOCAL_MIXED_PORT, "1.1.1.1", "/cdn-cgi/trace", 6000)
        } else {
            httpsGetDirect("https://1.1.1.1/cdn-cgi/trace", 5000)
        } ?: return@withContext null

        if (resp.statusCode != 200) return@withContext null
        val kv = EndpointRacer.parseTrace(resp.body)
        val warp = kv["warp"].orEmpty()
        TraceResult(
            colo = kv["colo"].orEmpty(),
            country = kv["loc"].orEmpty(),
            warpActive = warp == "on" || warp == "plus",
            rttMs = resp.latencyMs.toInt().coerceAtLeast(1),
        )
    }

    private fun probeTarget(t: Target, viaTunnel: Boolean): ProbeItem {
        val resp = if (viaTunnel) {
            httpsGetViaConnectProxy(SingBoxConfigBuilder.LOCAL_MIXED_PORT, t.host, t.path, 8500)
        } else {
            httpsGetDirect("https://${t.host}${t.path}", 6000)
        }
        if (resp == null) {
            return ProbeItem(name = t.name, reachable = false, latencyMs = 0, detail = "Ulaşılamadı")
        }
        val ok = resp.statusCode in 200..399
        return ProbeItem(
            name = t.name,
            reachable = ok,
            latencyMs = resp.latencyMs,
            detail = "HTTP ${resp.statusCode}",
        )
    }

    internal fun httpsGetViaConnectProxy(
        proxyPort: Int,
        host: String,
        path: String,
        timeoutMs: Int,
    ): SimpleHttpResponse? {
        val start = System.nanoTime()
        var rawSock: Socket? = null
        var tlsSock: SSLSocket? = null
        return try {
            rawSock = Socket().apply {
                soTimeout = timeoutMs
                connect(InetSocketAddress("127.0.0.1", proxyPort), 2500)
            }
            val connectReq = "CONNECT $host:443 HTTP/1.1\r\nHost: $host:443\r\nProxy-Connection: keep-alive\r\n\r\n"
            rawSock.getOutputStream().write(connectReq.toByteArray(Charsets.US_ASCII))
            rawSock.getOutputStream().flush()

            val rawIn = BufferedInputStream(rawSock.getInputStream())
            val connectHead = readHeadersOnly(rawIn)
            if (!connectHead.startsWith("HTTP/1.1 200") && !connectHead.startsWith("HTTP/1.0 200")) {
                return null
            }

            val sslFactory = SSLSocketFactory.getDefault() as SSLSocketFactory
            tlsSock = (sslFactory.createSocket(rawSock, host, 443, true) as SSLSocket).apply {
                soTimeout = timeoutMs
                startHandshake()
            }
            val verifier = HttpsURLConnection.getDefaultHostnameVerifier()
            if (!verifier.verify(host, tlsSock.session)) {
                return null
            }

            val req = "GET $path HTTP/1.1\r\nHost: $host\r\nUser-Agent: OzeVPN-Android/6.2\r\nAccept: */*\r\nConnection: close\r\n\r\n"
            tlsSock.outputStream.write(req.toByteArray(Charsets.US_ASCII))
            tlsSock.outputStream.flush()

            val tlsIn = BufferedInputStream(tlsSock.inputStream)
            val head = readHeadersOnly(tlsIn)
            val statusCode = parseStatusCode(head)
            if (statusCode <= 0) return null

            val body = if (path == "/cdn-cgi/trace") readBodyLimited(tlsIn, 4096) else ""
            val ms = ((System.nanoTime() - start) / 1_000_000L).coerceAtLeast(1)
            SimpleHttpResponse(statusCode = statusCode, body = body, latencyMs = ms)
        } catch (_: Throwable) {
            null
        } finally {
            runCatching { tlsSock?.close() }
            runCatching { rawSock?.close() }
        }
    }

    private fun httpsGetDirect(urlStr: String, timeoutMs: Int): SimpleHttpResponse? {
        val start = System.nanoTime()
        var conn: HttpURLConnection? = null
        return try {
            conn = (URL(urlStr).openConnection() as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = timeoutMs
                readTimeout = timeoutMs
                instanceFollowRedirects = false
                setRequestProperty("User-Agent", "OzeVPN-Android/6.2")
            }
            val code = conn.responseCode
            val body = if (urlStr.endsWith("/cdn-cgi/trace") && code == 200) {
                conn.inputStream.bufferedReader(Charsets.UTF_8).use { it.readText() }
            } else {
                ""
            }
            val ms = ((System.nanoTime() - start) / 1_000_000L).coerceAtLeast(1)
            SimpleHttpResponse(statusCode = code, body = body, latencyMs = ms)
        } catch (_: Throwable) {
            null
        } finally {
            conn?.disconnect()
        }
    }

    private fun readHeadersOnly(input: BufferedInputStream): String {
        val buf = ByteArrayOutputStream(512)
        var state = 0
        var count = 0
        while (count < 8192) {
            val b = input.read()
            if (b == -1) break
            buf.write(b)
            count++
            state = when {
                state == 0 && b == '\r'.code -> 1
                state == 1 && b == '\n'.code -> 2
                state == 2 && b == '\r'.code -> 3
                state == 3 && b == '\n'.code -> return buf.toString(Charsets.ISO_8859_1.name())
                b == '\r'.code -> 1
                else -> 0
            }
        }
        return buf.toString(Charsets.ISO_8859_1.name())
    }

    private fun readBodyLimited(input: BufferedInputStream, maxBytes: Int): String {
        val buf = ByteArrayOutputStream(maxBytes)
        val tmp = ByteArray(1024)
        var total = 0
        while (total < maxBytes) {
            val n = input.read(tmp, 0, minOf(tmp.size, maxBytes - total))
            if (n <= 0) break
            buf.write(tmp, 0, n)
            total += n
        }
        return buf.toString(Charsets.UTF_8.name())
    }

    private fun parseStatusCode(headers: String): Int {
        val firstLine = headers.lineSequence().firstOrNull().orEmpty()
        val parts = firstLine.split(' ')
        return if (parts.size >= 2) parts[1].trim().toIntOrNull() ?: 0 else 0
    }
}
