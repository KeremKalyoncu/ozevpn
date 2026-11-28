package com.keremkalyoncu.ozevpn.data

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject

data class WarpProfile(
    val privateKey: String,
    val addressV4: String,
    val addressV6: String,
    val peerPublicKey: String,
    val endpoint: String,
    val port: Int,
    val reserved: List<Int>,
    val lastRttMs: Int = 0,
    val lastColo: String = "",
    val lastProbedEpochSec: Long = 0L,
)

class PreferencesStore(context: Context) {
    private val prefs = context.applicationContext.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)

    fun loadMode(): RoutingMode = RoutingMode.fromKey(prefs.getString(KEY_MODE, RoutingMode.FULL.key))

    fun saveMode(mode: RoutingMode) {
        prefs.edit().putString(KEY_MODE, mode.key).apply()
    }

    fun loadAdBlock(): Boolean = prefs.getBoolean(KEY_ADBLOCK, true)

    fun saveAdBlock(enabled: Boolean) {
        prefs.edit().putBoolean(KEY_ADBLOCK, enabled).apply()
    }

    fun loadWarpProfile(): WarpProfile? {
        val raw = prefs.getString(KEY_WARP_PROFILE, null) ?: return null
        return runCatching {
            val o = JSONObject(raw)
            val priv = o.optString("private_key", "")
            val v4 = o.optString("address_v4", "")
            val pub = o.optString("peer_public_key", "")
            if (priv.isBlank() || v4.isBlank() || pub.isBlank()) return null
            val resArr = o.optJSONArray("reserved")
            val reserved = buildList {
                if (resArr != null) {
                    for (i in 0 until resArr.length()) add(resArr.optInt(i))
                }
            }
            WarpProfile(
                privateKey = priv,
                addressV4 = v4,
                addressV6 = o.optString("address_v6", ""),
                peerPublicKey = pub,
                endpoint = o.optString("endpoint", "162.159.192.1").ifBlank { "162.159.192.1" },
                port = o.optInt("port", 2408).takeIf { it in 1..65535 } ?: 2408,
                reserved = reserved,
                lastRttMs = o.optInt("last_rtt_ms", 0),
                lastColo = o.optString("last_colo", ""),
                lastProbedEpochSec = o.optLong("last_probed_sec", 0L),
            )
        }.getOrNull()
    }

    fun saveWarpProfile(profile: WarpProfile) {
        val o = JSONObject().apply {
            put("private_key", profile.privateKey)
            put("address_v4", profile.addressV4)
            put("address_v6", profile.addressV6)
            put("peer_public_key", profile.peerPublicKey)
            put("endpoint", profile.endpoint)
            put("port", profile.port)
            put("reserved", JSONArray(profile.reserved))
            put("last_rtt_ms", profile.lastRttMs)
            put("last_colo", profile.lastColo)
            put("last_probed_sec", profile.lastProbedEpochSec)
        }
        prefs.edit().putString(KEY_WARP_PROFILE, o.toString()).apply()
    }

    companion object {
        private const val PREF_NAME = "ozevpn_prefs"
        private const val KEY_MODE = "routing_mode"
        private const val KEY_ADBLOCK = "ad_block_enabled"
        private const val KEY_WARP_PROFILE = "warp_wg_profile_v1"
    }
}
