package com.keremkalyoncu.ozevpn.core

import com.keremkalyoncu.ozevpn.data.WarpProfile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import java.io.File
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket

data class RaceOutcome(
    val endpoint: String,
    val port: Int,
    val rttMs: Int,
    val colo: String,
    val country: String,
)

/**
 * Races all 8 official Cloudflare WARP Anycast endpoints simultaneously inside a single
 * ephemeral libsingbox.so process using direct HTTP CONNECT + TLS to 1.1.1.1:443,
 * selecting the lowest-RTT reachable POP with zero local ISP DNS leakage.
 */
object EndpointRacer {
    private const val FAST_CACHE_TTL_SEC = 1800L
    private const val FAST_CACHE_MAX_RTT_MS = 95

    suspend fun selectBestEndpoint(
        singBoxBinary: File,
        workDir: File,
        profile: WarpProfile,
    ): RaceOutcome? = withContext(Dispatchers.IO) {
        val nowSec = System.currentTimeMillis() / 1000L
        if (profile.lastProbedEpochSec > 0 &&
            (nowSec - profile.lastProbedEpochSec) < FAST_CACHE_TTL_SEC &&
            profile.lastRttMs in 1..FAST_CACHE_MAX_RTT_MS
        ) {
            return@withContext RaceOutcome(
                endpoint = profile.endpoint,
                port = profile.port,
                rttMs = profile.lastRttMs,
                colo = profile.lastColo,
                country = "",
            )
        }

        val candidates = WarpRegistrar.defaultCandidates
        val ports = reserveFreeLoopbackPorts(candidates.size)
        if (ports.size != candidates.size) return@withContext null

        val cfgFile = File(workDir, "probe-multi.json")
        cfgFile.writeText(SingBoxConfigBuilder.buildMultiProbeConfig(profile, candidates, ports), Charsets.UTF_8)

        var proc: Process? = null
        try {
            proc = ProcessBuilder(singBoxBinary.absolutePath, "run", "-c", cfgFile.absolutePath)
                .directory(workDir)
                .redirectErrorStream(true)
                .start()

            var ready = false
            for (i in 0 until 24) {
                if (isPortOpen(ports.first())) {
                    ready = true
                    break
                }
                Thread.sleep(75)
            }
            if (!ready) return@withContext null

            val results = withTimeoutOrNull(4500L) {
                coroutineScope {
                    candidates.mapIndexed { idx, cand ->
                        async(Dispatchers.IO) {
                            probeOne(cand, ports[idx])
                        }
                    }.awaitAll().filterNotNull()
                }
            }.orEmpty()

            results.minByOrNull { it.rttMs }
        } catch (_: Throwable) {
            null
        } finally {
            proc?.destroyForcibly()
            cfgFile.delete()
        }
    }

    private fun probeOne(cand: WarpEndpointCandidate, proxyPort: Int): RaceOutcome? {
        val resp = ConnectivityTester.httpsGetViaConnectProxy(
            proxyPort = proxyPort,
            host = "1.1.1.1",
            path = "/cdn-cgi/trace",
            timeoutMs = 3400,
        ) ?: return null
        if (resp.statusCode != 200) return null
        val kv = parseTrace(resp.body)
        val warpState = kv["warp"].orEmpty()
        if (warpState != "on" && warpState != "plus") return null
        return RaceOutcome(
            endpoint = cand.host,
            port = cand.port,
            rttMs = resp.latencyMs.toInt().coerceAtLeast(1),
            colo = kv["colo"].orEmpty(),
            country = kv["loc"].orEmpty(),
        )
    }

    internal fun parseTrace(body: String): Map<String, String> =
        body.lineSequence()
            .mapNotNull { line ->
                val idx = line.indexOf('=')
                if (idx > 0) line.substring(0, idx).trim() to line.substring(idx + 1).trim() else null
            }
            .toMap()

    private fun reserveFreeLoopbackPorts(count: Int): List<Int> {
        val sockets = mutableListOf<ServerSocket>()
        return try {
            repeat(count) {
                sockets.add(ServerSocket(0, 1, java.net.InetAddress.getByName("127.0.0.1")))
            }
            sockets.map { it.localPort }
        } catch (_: Throwable) {
            emptyList()
        } finally {
            sockets.forEach { runCatching { it.close() } }
        }
    }

    private fun isPortOpen(port: Int): Boolean = runCatching {
        Socket().use { s ->
            s.connect(InetSocketAddress("127.0.0.1", port), 120)
            true
        }
    }.getOrDefault(false)
}
