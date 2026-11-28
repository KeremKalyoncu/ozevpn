package com.keremkalyoncu.ozevpn.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

val ObsidianCanvas = Color(0xFF070B10)
val ObsidianSurface = Color(0xFF0E151E)
val ObsidianSurface2 = Color(0xFF151F2B)
val ObsidianBorder = Color(0x2694A3B8)

val LynxEmerald = Color(0xFF10B981)
val LynxEmeraldBright = Color(0xFF34D399)
val LynxEmeraldSoft = Color(0x2410B981)

val LynxAmber = Color(0xFFF59E0B)
val LynxAmberSoft = Color(0x26F59E0B)

val LynxDanger = Color(0xFFFB7185)
val LynxDangerSoft = Color(0x24F43F5E)

val LynxSlate = Color(0xFF64748B)
val TextPrimary = Color(0xFFEEF4F2)
val TextSecondary = Color(0xFF98A8B8)
val TextTertiary = Color(0xFF6B7D92)

private val OzeDarkColors = darkColorScheme(
    primary = LynxEmerald,
    onPrimary = Color(0xFF04130D),
    secondary = LynxEmeraldBright,
    background = ObsidianCanvas,
    onBackground = TextPrimary,
    surface = ObsidianSurface,
    onSurface = TextPrimary,
    surfaceVariant = ObsidianSurface2,
    onSurfaceVariant = TextSecondary,
    error = LynxDanger,
    outline = ObsidianBorder,
)

@Composable
fun OzeVpnTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = OzeDarkColors,
        content = content,
    )
}
