# OzeVPN'e Katkı Rehberi (Contributing Guide)

OzeVPN'e katkıda bulunmak istediğin için teşekkürler! Bu proje **sıfır harici Go/npm bağımlılığı**, sade okunabilir kod ve doğrulanabilir güvenlik ilkeleri üzerine kuruludur.

## Geliştirme İlkeleri
1. **Sıfır Harici Bağımlılık:** `go.mod` yalnızca Go standart kütüphanesini kullanır; `web/` dizini derleyici/bundler olmadan saf HTML5 + CSS3 + Vanilla JS (`<= 120 KB`) olarak kalır.
2. **Standart Go Düzeni:** Giriş noktası `cmd/ozevpn`, iç paketler `internal/<domain>`, statik gömme kaynakları `assets/` ve `web/` altındadır.
3. **Güvenlik Önce Gelir:** Yeni bir API uç noktası, özel sunucu ayrıştırıcısı veya ağ kuralı ekliyorsan mutlaka negatif güvenlik testlerini (`internal/server/server_test.go`, `internal/engine/engine_test.go`) birlikte ekle.

## Pull Request Öncesi Zorunlu Kontroller
```powershell
gofmt -l .
go vet ./...
go test -race -cover ./...
node --check web/app.js
node --check web/theme.js
```

## Güvenlik Açığı Bildirimi
Güvenlik açıklarını herkese açık Issue olarak açmayın; lütfen [SECURITY.md](SECURITY.md) dosyasındaki yönergeyi izleyin.
