package com.keremkalyoncu.ozevpn.ui

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Bolt
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Public
import androidx.compose.material.icons.rounded.Shield
import androidx.compose.material.icons.rounded.Speed
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.ColorMatrix
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.keremkalyoncu.ozevpn.R
import com.keremkalyoncu.ozevpn.data.ProbeItem
import com.keremkalyoncu.ozevpn.data.RoutingMode
import com.keremkalyoncu.ozevpn.data.TunnelPhase
import com.keremkalyoncu.ozevpn.data.TunnelSnapshot
import com.keremkalyoncu.ozevpn.ui.theme.LynxAmber
import com.keremkalyoncu.ozevpn.ui.theme.LynxAmberSoft
import com.keremkalyoncu.ozevpn.ui.theme.LynxDanger
import com.keremkalyoncu.ozevpn.ui.theme.LynxDangerSoft
import com.keremkalyoncu.ozevpn.ui.theme.LynxEmerald
import com.keremkalyoncu.ozevpn.ui.theme.LynxEmeraldBright
import com.keremkalyoncu.ozevpn.ui.theme.LynxEmeraldSoft
import com.keremkalyoncu.ozevpn.ui.theme.LynxSlate
import com.keremkalyoncu.ozevpn.ui.theme.ObsidianBorder
import com.keremkalyoncu.ozevpn.ui.theme.ObsidianCanvas
import com.keremkalyoncu.ozevpn.ui.theme.ObsidianSurface
import com.keremkalyoncu.ozevpn.ui.theme.ObsidianSurface2
import com.keremkalyoncu.ozevpn.ui.theme.TextPrimary
import com.keremkalyoncu.ozevpn.ui.theme.TextSecondary
import com.keremkalyoncu.ozevpn.ui.theme.TextTertiary

@Composable
fun OzeVpnScreen(
    snapshot: TunnelSnapshot,
    onTogglePower: () -> Unit,
    onSelectMode: (RoutingMode) -> Unit,
    onToggleAdBlock: (Boolean) -> Unit,
    onRunProbe: () -> Unit,
) {
    val accentColor by animateColorAsState(
        targetValue = when (snapshot.phase) {
            TunnelPhase.CONNECTED -> LynxEmerald
            TunnelPhase.CONNECTING -> LynxAmber
            TunnelPhase.ERROR -> LynxDanger
            TunnelPhase.DISCONNECTED -> LynxSlate
        },
        animationSpec = tween(320),
        label = "accent",
    )

    val topGlow = when (snapshot.phase) {
        TunnelPhase.CONNECTED -> LynxEmeraldSoft
        TunnelPhase.CONNECTING -> LynxAmberSoft
        TunnelPhase.ERROR -> LynxDangerSoft
        TunnelPhase.DISCONNECTED -> Color(0x1410B981)
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(
                Brush.verticalGradient(
                    colors = listOf(topGlow, ObsidianCanvas, ObsidianCanvas),
                    startY = 0f,
                    endY = 900f,
                )
            )
            .statusBarsPadding()
            .navigationBarsPadding(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 20.dp, vertical = 14.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            TopBrandHeader(snapshot = snapshot, accentColor = accentColor)

            LynxHeroCard(
                snapshot = snapshot,
                accentColor = accentColor,
                onTogglePower = onTogglePower,
            )

            QuickControlsCard(
                snapshot = snapshot,
                onSelectMode = onSelectMode,
                onToggleAdBlock = onToggleAdBlock,
            )

            AccessCheckCard(
                snapshot = snapshot,
                onRunProbe = onRunProbe,
            )

            Text(
                text = "Sıfır telemetri · WireGuard (TCP+UDP) + Şifreli DoH DNS",
                color = TextTertiary,
                fontSize = 11.5.sp,
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(top = 2.dp, bottom = 6.dp),
            )
        }
    }
}

@Composable
private fun TopBrandHeader(snapshot: TunnelSnapshot, accentColor: Color) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                modifier = Modifier
                    .size(38.dp)
                    .clip(RoundedCornerShape(11.dp))
                    .background(ObsidianSurface2)
                    .border(1.dp, accentColor.copy(alpha = 0.55f), RoundedCornerShape(11.dp)),
                contentAlignment = Alignment.Center,
            ) {
                Image(
                    painter = painterResource(id = R.drawable.lynx_mascot),
                    contentDescription = null,
                    modifier = Modifier.size(32.dp).clip(RoundedCornerShape(8.dp)),
                )
            }
            Spacer(modifier = Modifier.width(10.dp))
            Column {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        text = "OzeVPN",
                        color = TextPrimary,
                        fontSize = 18.sp,
                        fontWeight = FontWeight.Bold,
                        letterSpacing = (-0.3).sp,
                    )
                    Spacer(modifier = Modifier.width(7.dp))
                    Surface(
                        color = LynxEmeraldSoft,
                        shape = CircleShape,
                        border = BorderStroke(1.dp, LynxEmerald.copy(alpha = 0.45f)),
                    ) {
                        Text(
                            text = "v6.2",
                            color = LynxEmeraldBright,
                            fontSize = 10.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            fontFamily = FontFamily.Monospace,
                            modifier = Modifier.padding(horizontal = 7.dp, vertical = 2.dp),
                        )
                    }
                }
                Text(
                    text = "Anadolu Vaşağı",
                    color = TextTertiary,
                    fontSize = 11.5.sp,
                )
            }
        }

        Surface(
            color = ObsidianSurface,
            shape = CircleShape,
            border = BorderStroke(1.dp, ObsidianBorder),
        ) {
            Row(
                modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    modifier = Modifier
                        .size(7.dp)
                        .clip(CircleShape)
                        .background(accentColor)
                )
                Spacer(modifier = Modifier.width(6.dp))
                Text(
                    text = when (snapshot.phase) {
                        TunnelPhase.CONNECTED -> "Aktif"
                        TunnelPhase.CONNECTING -> "Kuruluyor"
                        TunnelPhase.ERROR -> "Hata"
                        TunnelPhase.DISCONNECTED -> "Hazır"
                    },
                    color = TextSecondary,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                )
            }
        }
    }
}

@Composable
private fun LynxHeroCard(
    snapshot: TunnelSnapshot,
    accentColor: Color,
    onTogglePower: () -> Unit,
) {
    val isConnected = snapshot.phase == TunnelPhase.CONNECTED
    val isConnecting = snapshot.phase == TunnelPhase.CONNECTING

    val infiniteTransition = rememberInfiniteTransition(label = "lynxPulse")
    val pulseScale by infiniteTransition.animateFloat(
        initialValue = 1.0f,
        targetValue = if (isConnecting) 1.09f else if (isConnected) 1.04f else 1.0f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = if (isConnecting) 900 else 2200, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "pulseScale",
    )

    val mascotFilter = if (snapshot.phase == TunnelPhase.DISCONNECTED) {
        ColorFilter.colorMatrix(ColorMatrix().apply { setToSaturation(0.30f) })
    } else {
        null
    }

    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = ObsidianSurface,
        shape = RoundedCornerShape(26.dp),
        border = BorderStroke(
            width = if (isConnected || isConnecting) 1.4.dp else 1.dp,
            color = if (isConnected || isConnecting) accentColor.copy(alpha = 0.52f) else ObsidianBorder,
        ),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 18.dp, vertical = 24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            // Big Tactile Anatolian Lynx Tap Target
            Box(
                modifier = Modifier.size(182.dp),
                contentAlignment = Alignment.Center,
            ) {
                Box(
                    modifier = Modifier
                        .size(172.dp)
                        .scale(pulseScale)
                        .clip(RoundedCornerShape(46.dp))
                        .background(accentColor.copy(alpha = if (isConnected || isConnecting) 0.14f else 0.05f))
                        .border(
                            width = 2.dp,
                            color = accentColor.copy(alpha = if (isConnected || isConnecting) 0.55f else 0.22f),
                            shape = RoundedCornerShape(46.dp),
                        ),
                )

                val ctaDesc = if (isConnected) "Bağlantıyı kes" else if (isConnecting) "Bağlanıyor" else "Bağlan"
                Box(
                    modifier = Modifier
                        .size(152.dp)
                        .clip(RoundedCornerShape(40.dp))
                        .background(ObsidianSurface2)
                        .border(
                            width = 2.dp,
                            color = accentColor.copy(alpha = if (isConnected || isConnecting) 0.85f else 0.35f),
                            shape = RoundedCornerShape(40.dp),
                        )
                        .semantics { contentDescription = ctaDesc }
                        .clickable(
                            role = Role.Button,
                            onClick = onTogglePower,
                        ),
                    contentAlignment = Alignment.Center,
                ) {
                    Image(
                        painter = painterResource(id = R.drawable.lynx_mascot),
                        contentDescription = null,
                        colorFilter = mascotFilter,
                        modifier = Modifier
                            .size(124.dp)
                            .clip(RoundedCornerShape(30.dp)),
                    )
                    if (isConnecting) {
                        CircularProgressIndicator(
                            color = LynxAmber,
                            strokeWidth = 3.dp,
                            modifier = Modifier.size(144.dp),
                        )
                    }
                }
            }

            Spacer(modifier = Modifier.height(16.dp))

            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.Center,
            ) {
                Text(
                    text = when (snapshot.phase) {
                        TunnelPhase.CONNECTED -> "Bağlı · Korunuyor"
                        TunnelPhase.CONNECTING -> "Bağlanıyor..."
                        TunnelPhase.ERROR -> "Bağlantı Hatası"
                        TunnelPhase.DISCONNECTED -> "Bağlı Değil"
                    },
                    color = TextPrimary,
                    fontSize = 22.sp,
                    fontWeight = FontWeight.Bold,
                    letterSpacing = (-0.4).sp,
                )
                if (isConnected && snapshot.uptimeSec > 0) {
                    Spacer(modifier = Modifier.width(8.dp))
                    Surface(
                        color = LynxEmeraldSoft,
                        shape = CircleShape,
                        border = BorderStroke(1.dp, LynxEmerald.copy(alpha = 0.45f)),
                    ) {
                        Text(
                            text = formatDuration(snapshot.uptimeSec),
                            color = LynxEmeraldBright,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.SemiBold,
                            fontFamily = FontFamily.Monospace,
                            modifier = Modifier.padding(horizontal = 9.dp, vertical = 3.dp),
                        )
                    }
                }
            }

            Spacer(modifier = Modifier.height(6.dp))

            Text(
                text = if (snapshot.phase == TunnelPhase.ERROR && snapshot.lastError.isNotBlank()) {
                    snapshot.lastError
                } else {
                    snapshot.statusDetail
                },
                color = if (snapshot.phase == TunnelPhase.ERROR) LynxDanger else TextSecondary,
                fontSize = 13.sp,
                textAlign = TextAlign.Center,
                lineHeight = 18.sp,
            )

            Spacer(modifier = Modifier.height(18.dp))

            // Crisp 3-Pill Telemetry Strip
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                val exitText = when {
                    snapshot.exitColo.isNotBlank() && snapshot.exitCountry.isNotBlank() ->
                        "${snapshot.exitColo} · ${snapshot.exitCountry}"
                    snapshot.exitColo.isNotBlank() -> snapshot.exitColo
                    else -> "—"
                }
                MetricPill(
                    label = "Çıkış",
                    value = exitText,
                    modifier = Modifier.weight(1f),
                )
                MetricPill(
                    label = "Gecikme",
                    value = if (snapshot.rttMs > 0 && isConnected) "${snapshot.rttMs} ms" else "—",
                    modifier = Modifier.weight(0.85f),
                )
                MetricPill(
                    label = "Canlı Hız",
                    value = if (isConnected) "↓ ${formatRateCompact(snapshot.rxBytesPerSec)}" else "—",
                    modifier = Modifier.weight(1.15f),
                )
            }
        }
    }
}

@Composable
private fun MetricPill(label: String, value: String, modifier: Modifier = Modifier) {
    Surface(
        modifier = modifier,
        color = ObsidianSurface2,
        shape = RoundedCornerShape(12.dp),
        border = BorderStroke(1.dp, ObsidianBorder),
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 8.dp),
        ) {
            Text(text = label, color = TextTertiary, fontSize = 10.5.sp)
            Spacer(modifier = Modifier.height(2.dp))
            Text(
                text = value,
                color = TextPrimary,
                fontSize = 12.5.sp,
                fontWeight = FontWeight.SemiBold,
                fontFamily = FontFamily.Monospace,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
private fun QuickControlsCard(
    snapshot: TunnelSnapshot,
    onSelectMode: (RoutingMode) -> Unit,
    onToggleAdBlock: (Boolean) -> Unit,
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = ObsidianSurface,
        shape = RoundedCornerShape(22.dp),
        border = BorderStroke(1.dp, ObsidianBorder),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = "Bağlantı Modu",
                    color = TextPrimary,
                    fontSize = 14.5.sp,
                    fontWeight = FontWeight.Bold,
                )
                Text(
                    text = "Anlık geçiş",
                    color = TextTertiary,
                    fontSize = 11.5.sp,
                )
            }

            // 2-Segment Mode Selector
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(14.dp))
                    .background(ObsidianCanvas)
                    .border(1.dp, ObsidianBorder, RoundedCornerShape(14.dp))
                    .padding(4.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                ModeSegmentButton(
                    title = "Tam Koruma",
                    selected = snapshot.mode == RoutingMode.FULL,
                    icon = Icons.Rounded.Public,
                    onClick = { onSelectMode(RoutingMode.FULL) },
                    modifier = Modifier.weight(1f),
                )
                ModeSegmentButton(
                    title = "Hibrit TR",
                    selected = snapshot.mode == RoutingMode.HYBRID_TR,
                    icon = Icons.Rounded.Bolt,
                    onClick = { onSelectMode(RoutingMode.HYBRID_TR) },
                    modifier = Modifier.weight(1f),
                )
            }

            Text(
                text = if (snapshot.mode == RoutingMode.FULL) {
                    "Roblox, Discord ve tüm internet tek dokunuşla şifreli tünelden açılır."
                } else {
                    "e-Devlet ve TR bankaları normal hatta kalır; Discord, Roblox ve kalan her şey tünelden akar."
                },
                color = TextSecondary,
                fontSize = 12.sp,
                lineHeight = 17.sp,
            )

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(1.dp)
                    .background(ObsidianBorder),
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Row(
                    modifier = Modifier.weight(1f).padding(end = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(
                        imageVector = Icons.Rounded.Shield,
                        contentDescription = null,
                        tint = LynxEmeraldBright,
                        modifier = Modifier.size(20.dp),
                    )
                    Spacer(modifier = Modifier.width(10.dp))
                    Column {
                        Text(
                            text = "Reklam & Takipçi Kalkanı",
                            color = TextPrimary,
                            fontSize = 13.5.sp,
                            fontWeight = FontWeight.SemiBold,
                        )
                        Text(
                            text = "28 reklam/izleyici ağını DNS katmanında engeller",
                            color = TextSecondary,
                            fontSize = 11.5.sp,
                        )
                    }
                }
                Switch(
                    checked = snapshot.adBlockEnabled,
                    onCheckedChange = onToggleAdBlock,
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = Color.White,
                        checkedTrackColor = LynxEmerald,
                        uncheckedThumbColor = TextSecondary,
                        uncheckedTrackColor = ObsidianSurface2,
                    ),
                )
            }
        }
    }
}

@Composable
private fun ModeSegmentButton(
    title: String,
    selected: Boolean,
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Surface(
        modifier = modifier
            .clip(RoundedCornerShape(10.dp))
            .clickable(role = Role.RadioButton, onClick = onClick),
        color = if (selected) LynxEmeraldSoft else Color.Transparent,
        shape = RoundedCornerShape(10.dp),
        border = if (selected) BorderStroke(1.dp, LynxEmerald.copy(alpha = 0.55f)) else null,
    ) {
        Row(
            modifier = Modifier.padding(vertical = 10.dp, horizontal = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.Center,
        ) {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = if (selected) LynxEmeraldBright else TextSecondary,
                modifier = Modifier.size(16.dp),
            )
            Spacer(modifier = Modifier.width(6.dp))
            Text(
                text = title,
                color = if (selected) LynxEmeraldBright else TextSecondary,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
            )
        }
    }
}

@Composable
private fun AccessCheckCard(
    snapshot: TunnelSnapshot,
    onRunProbe: () -> Unit,
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = ObsidianSurface,
        shape = RoundedCornerShape(22.dp),
        border = BorderStroke(1.dp, ObsidianBorder),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = "Canlı Erişim Testi",
                        color = TextPrimary,
                        fontSize = 14.5.sp,
                        fontWeight = FontWeight.Bold,
                    )
                    Text(
                        text = "Discord, Roblox & interneti doğrula",
                        color = TextSecondary,
                        fontSize = 11.5.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Spacer(modifier = Modifier.width(8.dp))
                Surface(
                    modifier = Modifier
                        .clip(RoundedCornerShape(10.dp))
                        .clickable(enabled = !snapshot.probing, role = Role.Button, onClick = onRunProbe),
                    color = LynxEmeraldSoft,
                    shape = RoundedCornerShape(10.dp),
                    border = BorderStroke(1.dp, LynxEmerald.copy(alpha = 0.5f)),
                ) {
                    Row(
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        if (snapshot.probing) {
                            CircularProgressIndicator(
                                color = LynxEmeraldBright,
                                strokeWidth = 2.dp,
                                modifier = Modifier.size(14.dp),
                            )
                        } else {
                            Icon(
                                imageVector = Icons.Rounded.Speed,
                                contentDescription = null,
                                tint = LynxEmeraldBright,
                                modifier = Modifier.size(16.dp),
                            )
                        }
                        Spacer(modifier = Modifier.width(6.dp))
                        Text(
                            text = if (snapshot.probing) "Ölçülüyor" else "Test Et",
                            color = LynxEmeraldBright,
                            fontSize = 12.5.sp,
                            fontWeight = FontWeight.SemiBold,
                        )
                    }
                }
            }

            if (snapshot.probes.isNotEmpty()) {
                val rows = snapshot.probes.chunked(2)
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    rows.forEach { pair ->
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            pair.forEach { item ->
                                ProbeChip(item = item, modifier = Modifier.weight(1f))
                            }
                            if (pair.size == 1) Spacer(modifier = Modifier.weight(1f))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ProbeChip(item: ProbeItem, modifier: Modifier = Modifier) {
    val borderColor = if (item.reachable) LynxEmerald.copy(alpha = 0.45f) else LynxDanger.copy(alpha = 0.45f)
    Surface(
        modifier = modifier,
        color = ObsidianSurface2,
        shape = RoundedCornerShape(12.dp),
        border = BorderStroke(1.dp, borderColor),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    imageVector = if (item.reachable) Icons.Rounded.CheckCircle else Icons.Rounded.ErrorOutline,
                    contentDescription = null,
                    tint = if (item.reachable) LynxEmeraldBright else LynxDanger,
                    modifier = Modifier.size(15.dp),
                )
                Spacer(modifier = Modifier.width(6.dp))
                Text(
                    text = item.name,
                    color = TextPrimary,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.SemiBold,
                )
            }
            Text(
                text = if (item.reachable) "${item.latencyMs} ms" else "Engelli",
                color = if (item.reachable) LynxEmeraldBright else LynxDanger,
                fontSize = 11.5.sp,
                fontFamily = FontFamily.Monospace,
                fontWeight = FontWeight.SemiBold,
            )
        }
    }
}

private fun formatDuration(sec: Long): String {
    val s = sec.coerceAtLeast(0L)
    val h = s / 3600
    val m = (s % 3600) / 60
    val r = s % 60
    return if (h > 0) String.format("%02d:%02d:%02d", h, m, r) else String.format("%02d:%02d", m, r)
}

private fun formatRateCompact(bps: Long): String = when {
    bps < 1024L -> "$bps B/s"
    bps < 1024L * 1024L -> String.format("%.0f KB/s", bps / 1024.0)
    else -> String.format("%.1f MB/s", bps / (1024.0 * 1024.0))
}
