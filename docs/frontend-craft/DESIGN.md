# OzeVPN v6 — Tasarım Anayasası (`DESIGN.md`)

## Kimlik
- **Kişilik:** *Atik, Kontrollü, Teknik*
- **Karşıtları:** *Dağınık, Kurumsal-Sıkıcı, Aşırı Neon / Emoji Çorbası*
- **Onaylı Görsel Yön:** **Yön B — Canlı Sinyal Kokpiti (*Signal Cockpit*)** — üstte tam genişlik Yatay Tünel Komuta Şeridi + gerçek veriye bağlı Canlı Tünel Rota Şeması; altında sade, kart enflasyonundan arındırılmış iki sütunlu çalışma alanı.

## Tipografi
- **Arayüz (Sans):** `'Segoe UI Variable', 'Segoe UI', system-ui, -apple-system, sans-serif` (0 harici font isteği, tam Türkçe glif desteği)
- **Ağ & Telemetri (Mono):** `'Cascadia Mono', 'JetBrains Mono', Consolas, ui-monospace, monospace` — tüm IP, port, domain, gecikme (`ms`), hız (`Mbps`) ve çalışma süresi (`tabular-nums`) verileri için zorunlu.
- **Ölçek:** `h1` `1.25rem` (700, `-0.02em`), Kokpit Durum Başlığı `1.35rem` (700), Bölüm Başlığı `0.95rem` (650), Gövde `0.85rem` (`1.48` line-height), Mikro Etiket `0.72rem` (uppercase, `+0.06em`).

## Renk ve Yüzey
| Semantik Rol | Koyu Tema (Obsidian Cockpit) | Açık Tema (Cool Porcelain) | Kullanım Kuralı |
|---|---|---|---|
| `--bg-canvas` | `#080b10` | `#f2f5f9` | Ana arka plan (hafif üst atmosfer) |
| `--bg-surface` | `#0f141d` | `#ffffff` | Birincil panel yüzeyi |
| `--bg-surface-2` | `#151c28` | `#f7f9fc` | Etkileşimli satır / input / seçili sekme |
| `--bg-inset` | `#06080d` | `#edf2f7` | Günlük konsolu ve telemetri yuvası |
| `--border-subtle` | `rgba(148,163,184,0.14)` | `rgba(15,23,42,0.10)` | Panel kenarı ve hairline ayırıcılar |
| `--border-control` | `rgba(148,163,184,0.28)` | `rgba(15,23,42,0.22)` | Input, select ve ikincil buton sınırı |
| `--text-primary` | `#eef2f7` | `#0b1220` | Ana metin (AA+ yüksek kontrast) |
| `--text-secondary` | `#9aa7bc` | `#475569` | Yardımcı açıklama ve etiketler |
| `--signal-accent` | `#0ea5e9` | `#0284c7` | Birincil CTA, aktif mod ve sinyal bağlantısı |
| `--status-ok` | `#22c55e` | `#059669` | Yalnız aktif şifreli tünel ve erişilebilir hedef |
| `--status-warn` | `#f59e0b` | `#b45309` | Bağlanıyor / orta gecikme uyarısı |
| `--status-danger` | `#f43f5e` | `#be123c` | Hata, kopukluk ve yıkıcı işlem onayı |

- **Radius Ölçeği:** `--radius-sm: 6px`, `--radius-md: 10px`, `--radius-lg: 14px`, `--radius-pill: 999px`.
- **Spacing Ölçeği:** `4 / 8 / 12 / 16 / 20 / 24 px`.

## Layout
- **Üst Çubuk:** Tek satır `56px` — Solda kalkan logosu + `OzeVPN v6`; sağda `Oyun TUN`, `Tema` ve ikincil eylemleri toplayan erişilebilir `Araçlar` açılır menüsü.
- **Komuta Şeridi (Hero):** Tam genişlik yatay kokpit şeridi — Güç Çekirdeği + Canlı Tünel Rota Şeması (`Cihaz → Katman → Tünel → Çıkış`) + tek satır Ağ/DNS/IPv6 + `IP Gizle`.
- **Çalışma Alanı:** Masaüstünde `minmax(360px, 420px) minmax(0, 1fr)`; `<= 960px` tek sütun. Tüm flex/grid çocuklarında `min-width: 0; overflow-wrap: anywhere` zorunludur.
- **Dokunma Hedefleri:** Mobil/dokunmatik görünümde (`<= 720px`) tüm etkileşimli öğeler en az `44px` yüksekliktedir.

## Motion
- **Amaç:** Yalnız durum geçişi ve etkileşim geri bildirimi; süs animasyonu yok.
- **Timing:** Hızlı kontroller `140ms`, yüzey/menü geçişleri `220ms` (`cubic-bezier(0.2, 0, 0, 1)`).
- **Performans:** Yalnız GPU-composited `transform` ve `opacity` (yasak: `box-shadow` keyframe repaint).
- **Reduced Motion:** `prefers-reduced-motion: reduce` açıkken tüm animasyon ve geçişler kapatılır; statik durum göstergeleri eksiksiz kalır.
- **Signature Moment Sınırı:** Yalnızca üst Komuta Şeridi'ndeki Güç Çekirdeği halkası + Canlı Tünel Rota Şeması aktif durum geçişine sahiptir; geri kalan tüm arayüz sakin kalır.

## Etkileşim
- **Switch & Seçiciler:** Ham `13px` checkbox yerine erişilebilir `role="switch"` CSS anahtarları; iç içe 4 katmanlı kutu yerine hairline ayırıcılı satırlar.
- **Toast Bildirimleri:** Sağ alt (`safe-area` korumalı) `aria-live="polite"` yığın (en fazla 3 görünür, dedupe, başarı `3.8 sn`, hata kalıcı kapatmalı); kalıcı kayıt `#logBox`'ta saklanır.
- **Focus & Modal:** Modal kapanışında odak asla `.skip-link`'e düşmez; tetikleyiciye veya `#powerBtn`'e döner.
- **Klavye Kısayolları:** `Alt+C` (Bağlan/Kes), `Alt+T` (Canlı Test), `/` (Site Kurtarıcı odak — input dışındayken), `Escape` (modal/menü kapat).

## İçerik ve Varlık
- **Ses Tonu:** Net, dürüst, kısa Türkçe; sahte metrik, pazarlama sıfatı ve uzun parantez içi kalabalık yok.
- **İkon Ailesi:** Sıfır OS emojisi. Tek tutarlı Lucide-stil (`ISC` / özgün) `24×24`, `1.75px` stroke inline SVG sprite (`web/index.html`).

## Onaylanmış Zevk Kuralları (`Learn Taste`)
1. **Kapsam:** Genel görsel yön & yerleşim — **Kural:** *"Yön B (Canlı Sinyal Kokpiti) tercih edilir: daha etkileyici, daha sade ve net."* — **Neden:** Kullanıcı doğrudan bağlantı durumunu ve tünel akışını net, sade ve etkileyici biçimde görmek istiyor. — **Tarih / Kaynak:** `2026-11-27`, kullanıcı onayı. — **Otomatik test:** Edge CDP ekran görüntüsü + `emojiCount === 0` + `topbarHeight <= 72px` + `overflowX === false`.
2. **Kapsam:** Denetim kapsamı — **Kural:** `FE-26-04` (Checkout) ücretsiz yerel VPN uygulaması gereği `N/A` kapsam dışıdır. — **Tarih / Kaynak:** `2026-11-27`, kullanıcı onayı.
