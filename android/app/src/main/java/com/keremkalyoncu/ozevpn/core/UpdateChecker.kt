package com.keremkalyoncu.ozevpn.core

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

data class ReleaseUpdate(
    val version: String,
    val downloadUrl: String,
)

object UpdateChecker {
    private const val LATEST_RELEASE_API = "https://api.github.com/repos/KeremKalyoncu/ozevpn/releases/latest"
    private const val FALLBACK_RELEASES_URL = "https://github.com/KeremKalyoncu/ozevpn/releases/latest"

    suspend fun checkLatest(currentVersion: String): ReleaseUpdate? = withContext(Dispatchers.IO) {
        runCatching {
            val conn = (URL(LATEST_RELEASE_API).openConnection() as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = 4000
                readTimeout = 4000
                setRequestProperty("Accept", "application/vnd.github+json")
                setRequestProperty("User-Agent", "OzeVPN-Android/$currentVersion")
            }
            try {
                if (conn.responseCode !in 200..299) return@withContext null
                val body = conn.inputStream.bufferedReader().use { it.readText() }
                val json = JSONObject(body)
                val tag = json.optString("tag_name", "").trim()
                if (tag.isBlank() || !isNewerVersion(tag, currentVersion)) return@withContext null

                var apkUrl = ""
                val assets = json.optJSONArray("assets")
                if (assets != null) {
                    for (i in 0 until assets.length()) {
                        val asset = assets.optJSONObject(i) ?: continue
                        val name = asset.optString("name", "")
                        val url = asset.optString("browser_download_url", "")
                        if (name.equals("OzeVPN-Android-arm64.apk", ignoreCase = true) && url.startsWith("https://")) {
                            apkUrl = url
                            break
                        }
                    }
                }
                val htmlUrl = json.optString("html_url", "").takeIf { it.startsWith("https://") }
                    ?: FALLBACK_RELEASES_URL
                ReleaseUpdate(
                    version = tag.removePrefix("v").removePrefix("V"),
                    downloadUrl = apkUrl.ifBlank { htmlUrl },
                )
            } finally {
                conn.disconnect()
            }
        }.getOrNull()
    }

    internal fun isNewerVersion(candidate: String, current: String): Boolean {
        val a = parseSemver(candidate) ?: return false
        val b = parseSemver(current) ?: return false
        for (i in 0 until 3) {
            if (a[i] != b[i]) return a[i] > b[i]
        }
        return false
    }

    private fun parseSemver(raw: String): IntArray? {
        val clean = raw.trim().removePrefix("v").removePrefix("V").substringBefore("-")
        val parts = clean.split(".")
        if (parts.isEmpty() || parts.size > 3) return null
        val out = IntArray(3)
        for (i in parts.indices) {
            val v = parts[i].toIntOrNull() ?: return null
            if (v < 0) return null
            out[i] = v
        }
        return out
    }
}
