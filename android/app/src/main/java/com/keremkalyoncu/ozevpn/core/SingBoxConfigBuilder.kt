package com.keremkalyoncu.ozevpn.core

import com.keremkalyoncu.ozevpn.data.RoutingMode
import com.keremkalyoncu.ozevpn.data.WarpProfile
import org.json.JSONArray
import org.json.JSONObject

object SingBoxConfigBuilder {
    const val LOCAL_MIXED_PORT = 10808
    private const val MTU_WIREGUARD = 1280
    private const val WG_KEEPALIVE_SEC = 25

    val adTrackerDomains: List<String> = listOf(
        "doubleclick.net",
        "googlesyndication.com",
        "googleadservices.com",
        "google-analytics.com",
        "googletagmanager.com",
        "googletagservices.com",
        "adservice.google.com",
        "pagead2.googlesyndication.com",
        "ads.youtube.com",
        "adnxs.com",
        "adsrvr.org",
        "criteo.com",
        "outbrain.com",
        "taboola.com",
        "scorecardresearch.com",
        "quantserve.com",
        "rubiconproject.com",
        "pubmatic.com",
        "openx.net",
        "amazon-adsystem.com",
        "moatads.com",
        "hotjar.com",
        "mixpanel.com",
        "segment.io",
        "facebook.net",
        "pixel.facebook.com",
        "analytics.tiktok.com",
        "ads.twitter.com",
    )

    val trDirectBypassDomains: List<String> = listOf(
        "gov.tr",
        "edu.tr",
        "bel.tr",
        "k12.tr",
        "pol.tr",
        "ziraatbank.com.tr",
        "garantibbva.com.tr",
        "isbank.com.tr",
        "akbank.com",
        "yapikredi.com.tr",
        "qnbfinansbank.com",
        "enpara.com",
        "papara.com",
        "vakifbank.com.tr",
        "halkbank.com.tr",
        "turkiye.gov.tr",
    )

    fun buildTunnelConfig(
        profile: WarpProfile,
        mode: RoutingMode,
        adBlockEnabled: Boolean,
        listenPort: Int = LOCAL_MIXED_PORT,
    ): String {
        val addrs = JSONArray().apply {
            put(profile.addressV4)
            if (profile.addressV6.isNotBlank()) put(profile.addressV6)
        }
        val peer = JSONObject().apply {
            put("address", profile.endpoint)
            put("port", profile.port)
            put("public_key", profile.peerPublicKey)
            put("allowed_ips", JSONArray(listOf("0.0.0.0/0", "::/0")))
            put("persistent_keepalive_interval", WG_KEEPALIVE_SEC)
            if (profile.reserved.size == 3) {
                put("reserved", JSONArray(profile.reserved))
            }
        }
        val warpEndpoint = JSONObject().apply {
            put("type", "wireguard")
            put("tag", "warp-fast")
            put("mtu", MTU_WIREGUARD)
            put("address", addrs)
            put("private_key", profile.privateKey)
            put("peers", JSONArray().put(peer))
        }

        val routeRules = JSONArray().apply {
            put(JSONObject().put("action", "sniff"))
            put(JSONObject().put("protocol", "dns").put("action", "hijack-dns"))
            put(JSONObject().put("ip_is_private", true).put("outbound", "direct"))
            if (adBlockEnabled) {
                put(
                    JSONObject()
                        .put("domain_suffix", JSONArray(adTrackerDomains))
                        .put("action", "reject")
                )
            }
            if (mode == RoutingMode.HYBRID_TR) {
                put(
                    JSONObject()
                        .put("domain_suffix", JSONArray(trDirectBypassDomains))
                        .put("outbound", "direct")
                )
            }
        }

        val dnsRules = JSONArray().apply {
            if (mode == RoutingMode.HYBRID_TR) {
                put(
                    JSONObject()
                        .put("domain_suffix", JSONArray(trDirectBypassDomains))
                        .put("server", "dns-direct")
                )
            }
        }

        val root = JSONObject().apply {
            put("log", JSONObject().put("level", "warn").put("timestamp", true))
            put(
                "dns",
                JSONObject().apply {
                    put(
                        "servers",
                        JSONArray().apply {
                            put(
                                JSONObject()
                                    .put("tag", "dns-doh")
                                    .put("type", "https")
                                    .put("server", "1.1.1.1")
                                    .put("detour", "warp-fast")
                            )
                            put(
                                JSONObject()
                                    .put("tag", "dns-direct")
                                    .put("type", "https")
                                    .put("server", "1.1.1.1")
                            )
                        }
                    )
                    put("rules", dnsRules)
                    put("final", "dns-doh")
                    put("strategy", "prefer_ipv4")
                }
            )
            put(
                "inbounds",
                JSONArray().put(
                    JSONObject()
                        .put("type", "mixed")
                        .put("tag", "mixed-in")
                        .put("listen", "127.0.0.1")
                        .put("listen_port", listenPort)
                )
            )
            put("endpoints", JSONArray().put(warpEndpoint))
            put("outbounds", JSONArray().put(JSONObject().put("type", "direct").put("tag", "direct")))
            put(
                "route",
                JSONObject().apply {
                    put("rules", routeRules)
                    put("final", "warp-fast")
                    // Critical on non-root Android 11+: avoids NETLINK_ROUTE permission errors;
                    // VpnService.addDisallowedApplication(packageName) already bypasses TUN for us.
                    put("auto_detect_interface", false)
                    put("default_domain_resolver", "dns-direct")
                }
            )
        }
        return root.toString(2)
    }

    fun buildMultiProbeConfig(
        profile: WarpProfile,
        candidates: List<WarpEndpointCandidate>,
        ports: List<Int>,
    ): String {
        val addrs = JSONArray().apply {
            put(profile.addressV4)
            if (profile.addressV6.isNotBlank()) put(profile.addressV6)
        }
        val inbounds = JSONArray()
        val endpoints = JSONArray()
        val rules = JSONArray()

        candidates.forEachIndexed { idx, c ->
            val inTag = "in-$idx"
            val epTag = "ep-$idx"
            inbounds.put(
                JSONObject()
                    .put("type", "http")
                    .put("tag", inTag)
                    .put("listen", "127.0.0.1")
                    .put("listen_port", ports[idx])
            )
            val peer = JSONObject().apply {
                put("address", c.host)
                put("port", c.port)
                put("public_key", profile.peerPublicKey)
                put("allowed_ips", JSONArray(listOf("0.0.0.0/0", "::/0")))
                if (profile.reserved.size == 3) {
                    put("reserved", JSONArray(profile.reserved))
                }
            }
            endpoints.put(
                JSONObject()
                    .put("type", "wireguard")
                    .put("tag", epTag)
                    .put("mtu", MTU_WIREGUARD)
                    .put("address", addrs)
                    .put("private_key", profile.privateKey)
                    .put("peers", JSONArray().put(peer))
            )
            rules.put(JSONObject().put("inbound", JSONArray().put(inTag)).put("outbound", epTag))
        }

        return JSONObject().apply {
            put("log", JSONObject().put("level", "error"))
            put("inbounds", inbounds)
            put("endpoints", endpoints)
            put("outbounds", JSONArray().put(JSONObject().put("type", "direct").put("tag", "direct")))
            put(
                "route",
                JSONObject()
                    .put("rules", rules)
                    .put("final", "direct")
                    .put("auto_detect_interface", false)
            )
        }.toString()
    }

    fun buildHevTunnelYaml(listenPort: Int = LOCAL_MIXED_PORT): String = """
tunnel:
  name: tun0
  mtu: $MTU_WIREGUARD
  multi-queue: false
  ipv4: 10.233.233.1
  ipv6: 'fdfe:dcba:9876::1'
socks5:
  port: $listenPort
  address: 127.0.0.1
  udp: 'udp'
misc:
  task-stack-size: 81920
  tcp-buffer-size: 65536
  connect-timeout: 5000
  read-write-timeout: 60000
  log-level: warn
""".trimIndent() + "\n"
}
