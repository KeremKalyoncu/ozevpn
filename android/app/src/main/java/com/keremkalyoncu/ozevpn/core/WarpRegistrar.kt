package com.keremkalyoncu.ozevpn.core

import com.keremkalyoncu.ozevpn.data.PreferencesStore
import com.keremkalyoncu.ozevpn.data.WarpProfile
import org.json.JSONObject
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Base64

data class WarpEndpointCandidate(val host: String, val port: Int)

object WarpRegistrar {
    // Prioritize 188.114.96-99.1 pools which route to European POPs (unblocking Roblox & TR-geo-restricted sites);
    // keep 162.159.x.x as fallback only if all European pools are unreachable.
    val defaultCandidates: List<WarpEndpointCandidate> = listOf(
        WarpEndpointCandidate("188.114.97.1", 2408),
        WarpEndpointCandidate("188.114.96.1", 2408),
        WarpEndpointCandidate("188.114.98.1", 1701),
        WarpEndpointCandidate("188.114.99.1", 854),
        WarpEndpointCandidate("188.114.96.1", 500),
        WarpEndpointCandidate("188.114.97.1", 4500),
        WarpEndpointCandidate("162.159.195.1", 4500),
        WarpEndpointCandidate("162.159.192.1", 2408),
    )

    private val regUrls = listOf(
        "https://api.cloudflareclient.com/v0a2158/reg",
        "https://api.cloudflareclient.com/v0a2483/reg",
    )

    private val tosFormatter = DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss.000'Z'")
        .withZone(ZoneOffset.UTC)

    fun ensureProfile(store: PreferencesStore): WarpProfile {
        store.loadWarpProfile()?.let { return it }
        val fresh = registerFresh()
        store.saveWarpProfile(fresh)
        return fresh
    }

    fun registerFresh(): WarpProfile {
        val kp = X25519KeyGen.generateKeyPair()
        val payload = JSONObject().apply {
            put("key", kp.publicKeyBase64)
            put("install_id", "")
            put("fcm_token", "")
            put("tos", tosFormatter.format(Instant.now()))
            put("model", "Android")
            put("serial_number", "")
            put("locale", "en_US")
        }.toString()

        var lastError: Throwable? = null
        for (endpointUrl in regUrls) {
            var conn: HttpURLConnection? = null
            try {
                conn = (URL(endpointUrl).openConnection() as HttpURLConnection).apply {
                    requestMethod = "POST"
                    connectTimeout = 6000
                    readTimeout = 8000
                    doOutput = true
                    setRequestProperty("Content-Type", "application/json; charset=UTF-8")
                    setRequestProperty("User-Agent", "okhttp/3.12.1")
                    setRequestProperty("CF-Client-Version", "a-6.11-2223")
                }
                OutputStreamWriter(conn.outputStream, Charsets.UTF_8).use { it.write(payload) }
                val code = conn.responseCode
                val body = (if (code in 200..299) conn.inputStream else conn.errorStream)
                    ?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }.orEmpty()
                if (code != HttpURLConnection.HTTP_OK) {
                    lastError = IllegalStateException("Cloudflare kayıt HTTP $code")
                    continue
                }

                val cfg = JSONObject(body).optJSONObject("config")
                    ?: throw IllegalStateException("Cloudflare kayıt yanıtı eksik (config)")
                val peers = cfg.optJSONArray("peers")
                val peerPub = peers?.optJSONObject(0)?.optString("public_key", "")?.trim().orEmpty()
                val addrs = cfg.optJSONObject("interface")?.optJSONObject("addresses")
                var v4 = addrs?.optString("v4", "")?.trim().orEmpty()
                var v6 = addrs?.optString("v6", "")?.trim().orEmpty()
                if (peerPub.isBlank() || v4.isBlank()) {
                    throw IllegalStateException("Cloudflare kayıt yanıtı eksik (peer/v4)")
                }
                if (!v4.contains("/")) v4 = "$v4/32"
                if (v6.isNotBlank() && !v6.contains("/")) v6 = "$v6/128"

                val reserved = parseReservedBytes(cfg.optString("client_id", ""))
                val first = defaultCandidates.first()
                return WarpProfile(
                    privateKey = kp.privateKeyBase64,
                    addressV4 = v4,
                    addressV6 = v6,
                    peerPublicKey = peerPub,
                    endpoint = first.host,
                    port = first.port,
                    reserved = reserved,
                )
            } catch (t: Throwable) {
                lastError = t
            } finally {
                conn?.disconnect()
            }
        }
        throw IllegalStateException(
            "WARP kaydı başarısız: ${lastError?.message ?: "ağ bağlantısı yok"}",
            lastError,
        )
    }

    internal fun parseReservedBytes(clientIdBase64: String): List<Int> {
        val trimmed = clientIdBase64.trim()
        if (trimmed.isBlank()) return emptyList()
        return runCatching {
            val raw = Base64.getDecoder().decode(trimmed)
            if (raw.size == 3) raw.map { it.toInt() and 0xFF } else emptyList()
        }.getOrDefault(emptyList())
    }
}
