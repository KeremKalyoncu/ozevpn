# OzeVPN — Arayüz Tasarım Sistemi (`docs/DESIGN.md`)

## Kimlik
- **Görsel Dil:** *Signal Cockpit (Canlı Sinyal Kokpiti)* — üstte tam genişlik Yatay Tünel Komuta Şeridi ve gerçek veriye bağlı Canlı Tünel Rota Şeması; altında sade iki sütunlu kontrol alanı.
- **Kişilik:** Atik, kontrollü ve teknik; emoji karmaşası veya gereksiz kart katmanları kullanılmaz.

## Tipografi
- **Arayüz (Sans):** `'Segoe UI Variable', 'Segoe UI', system-ui, -apple-system, sans-serif` (harici font/CDN isteği yok, tam Türkçe glif desteği).
- **Ağ & Telemetri (Mono):** `'Cascadia Mono', 'JetBrains Mono', Consolas, ui-monospace, monospace` — IP, port, alan adı, gecikme (`ms`), hız (`Mbps`) ve çalışma süresi (`tabular-nums`) verileri için kullanılır.
- **Ölçek:** `h1` `1.25rem` (`700`, `-0.02em`), Kokpit Durum Başlığı `1.35rem` (`700`), Bölüm Başlığı `0.95rem` (`650`), Gövde `0.85rem` (`1.48` satır yüksekliği), Mikro Etiket `0.72rem` (uppercase, `+0.06em`).

## Renk ve Yüzey Token'ları
| Semantik Rol | Koyu Tema (Obsidian Cockpit) | Açık Tema (Cool Porcelain) | Kullanım Kuralı |
| :--- | :--- | :--- | :--- |
| `--bg-canvas` | `#080b10` | `#f2f5f9` | Ana arka plan |
| `--bg-surface` | `#0f141d` | `#ffffff` | Birincil panel yüzeyi |
| `--bg-surface-2` | `#151c28` | `#f7f9fc` | Etkileşimli satır / input / seçili sekme |
| `--bg-inset` | `#06080d` | `#edf2f7` | Günlük konsolu ve telemetri yuvası |
| `--border-subtle` | `rgba(148,163,184,0.14)` | `rgba(15,23,42,0.10)` | Panel kenarı ve hairline ayırıcılar |
| `--border-control` | `rgba(148,163,184,0.28)` | `rgba(15,23,42,0.22)` | Input, select ve ikincil buton sınırı |
| `--text-primary` | `#eef2f7` | `#0b1220` | Ana metin (WCAG AA+ kontrast) |
| `--text-secondary` | `#9aa7bc` | `#475569` | Yardımcı açıklama ve etiketler |
| `--signal-accent` | `#0ea5e9` | `#0284c7` | Birincil CTA, aktif mod ve sinyal bağlantısı |
| `--status-ok` | `#22c55e` | `#059669` | Aktif şifreli tünel ve erişilebilir hedef |
| `--status-warn` | `#f59e0b` | `#b45309` | Bağlanıyor / orta gecikme uyarısı |
| `--status-danger` | `#f43f5e` | `#be123c` | Hata, kopukluk ve yıkıcı işlem onayı |

- **Radius Ölçeği:** `--radius-sm: 6px`, `--radius-md: 10px`, `--radius-lg: 14px`, `--radius-pill: 999px`.
- **Spacing Ölçeği:** `4 / 8 / 12 / 16 / 20 / 24 px`.

## Yerleşim, Hareket ve Erişilebilirlik
- **Üst Çubuk:** Tek satır `47–56px` — solda kalkan logosu + `OzeVPN v6`; sağda `Oyun TUN`, tema düğmesi ve ikincil eylemleri toplayan erişilebilir `Araçlar` açılır menüsü.
- **Komuta Şeridi:** Güç Çekirdeği + Canlı Tünel Rota Şeması (`Cihaz → Katman → Tünel → Çıkış`) + tek satır Ağ/DNS/IPv6 + `IP Gizle`.
- **Responsive & Bütçe:** Masaüstünde `minmax(360px, 420px) minmax(0, 1fr)`, `<= 960px` tek sütun, mobil dokunma hedefleri `>= 44px`, toplam gömülü `web/*` varlıkları `<= 120 KB` (`TestWebUIBudgetAndAccessibilityContract`).
- **Motion:** Yalnız GPU-composited `transform` ve `opacity` (`140ms`/`220ms`); `prefers-reduced-motion: reduce` altında devre dışı kalır.
- **Etkileşim & Klavye:** Erişilebilir `role="switch"` anahtarları, en fazla 3 görünür `aria-live="polite"` toast, modal odak geri dönüşü (`#powerBtn`) ve klavye kısayolları (`Alt+C`, `Alt+T`, `/`, `Escape`).
- **İkonlar:** Tek tutarlı `24×24`, `1.75px` stroke inline SVG sprite (`web/index.html`), sıfır OS emojisi.
