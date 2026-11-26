## Değişiklik Özeti
- [ ] Kök neden ve kapsam açıkça belirtildi

## UI / UX & Erişilebilirlik Kontrol Listesi
- [ ] Mevcut koyu/açık tema marka kimliği ve WCAG 2.2 AA kontrastı (≥4.5:1 metin, ≥3:1 odak & sınır) korundu
- [ ] Klavye erişimi (`Tab`, `Escape`, modal odak tuzağı, sekme ok tuşları) ve görünür `:focus-visible` doğrulandı
- [ ] `320px`, `375px`, `768px`, `1024px`, `1440px` genişliklerde `0px` yatay taşma korundu
- [ ] `CLS ≤ 0.10` (asenkron kartlarda düzen kayması yok)
- [ ] `go vet ./...` ve `go test -v ./...` (UI bütçe & erişilebilirlik sözleşme testi dahil) geçti
- [ ] Gerekliyse tek dosya `.exe` (`.\build-single-exe.ps1`) yeniden derlendi
