## Özet
- Bu PR'da ne değiştirildi?

## VPN Güvenlik & Yayın Kapısı Kontrol Listesi
- [ ] `go vet ./...` ve `go test -race -cover ./...` temiz geçti
- [ ] `sing-box check` (masaüstü dual-stack TUN + OpenWrt export + WireGuard) geçti
- [ ] IPC oturum tokeni (`X-OzeVPN-Token`), CSRF/DNS-Rebinding ve NTFS DACL/DPAPI korumaları bozulmadı
- [ ] IPv6 / DNS / WebRTC / Kill Switch sızıntı politikaları korundu
- [ ] `BINARY_CHECKSUMS.txt` (5 çalışma zamanı dosyası) doğrulandı ve yeni gizli anahtar sızdırılmadı
- [ ] İkinci göz / bağımsız güvenlik incelemesi (kritik P0/P1 değişiklikleri için): `@reviewer` — Karar: `[ ] Onaylandı / [ ] N/A`

## UI / Erişilebilirlik Bütçesi
- [ ] Gömülü web varlıkları `<= 120 KB` bütçesi içinde
- [ ] Açık / Koyu tema ve WCAG 2.2 AA kontrast korundu
