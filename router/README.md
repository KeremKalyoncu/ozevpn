# OzeVPN Faz 2: Modemle Arana Giren Akıllı Router (OpenWrt)

PC, telefon, televizyon ve konsolların **hiçbirine tek tek VPN kurmadan** tüm evi sansürsüz ve daha stabil pingle çalıştırmanın en temiz yolu, ISP modeminin arkasına **OpenWrt** kurulu küçük bir router koymaktır.

## Fiziksel Bağlantı Şeması

```text
[Fiber / VDSL Hat] ---> [ISP Modemi (Köprü veya LAN)] ---> [OpenWrt Router (OzeVPN)] ---> [PC, Telefon, TV, Konsol]
```

## Bu Kutu İnterneti Nasıl "Hızlandırır"?
1. **SQM / CAKE (Bufferbloat Çözümü):** Evde biri 4K video izlerken veya Steam'den oyun indirirken pinginin 20 ms'den 300 ms'ye fırlamasını engeller. İnternet hissedilir şekilde akıcılaşır.
2. **Akıllı Split-Tunnel (`sing-box`):** Sadece Discord, Roblox ve seçtiğin engelli siteler tünelden geçer. Yerli siteler, bankalar ve indirmeler %100 direkt hızda çıkar.
3. **Şifreli DNS (DoH):** ISP DNS zehirlemesini ve gereksiz gecikmeleri ortadan kaldırır.

## Önerilen Uygun Fiyatlı OpenWrt Donanımları
- **Xiaomi Router AX3000T** (fiyat/performans şampiyonu, güçlü ARM CPU, WireGuard/sing-box'ta 300+ Mbps)
- **GL.iNet Flint 2 (MT6000)** veya **Beryl AX** (kutudan OpenWrt + kolay arayüzle gelir)
- **Raspberry Pi 4 / 5** veya eski bir **Mini PC (x86)** (OpenWrt x86 kurarsan 1 Gbps'ye kadar sıfır darboğaz)

## Kurulum Adımları
1. Router'a SSH ile bağlan:
   ```bash
   ssh root@192.168.1.1
   ```
2. `router/openwrt-setup.sh` dosyasını router'a kopyalayıp çalıştır:
   ```bash
   scp router/openwrt-setup.sh root@192.168.1.1:/tmp/
   ssh root@192.168.1.1 "sh /tmp/openwrt-setup.sh"
   ```
3. PC'deki **OzeVPN** arayüzünden **Modem / Router (Faz 2)** sekmesine girip `sing-box-openwrt.json` dosyasını indir.
4. İçindeki `private_key` alanına ücretsiz Cloudflare WARP WireGuard profilini (`wgcf` ile üretilebilir) veya ileride kiralayacağın kendi VPS anahtarını yaz.
5. Router'a kopyala ve başlat:
   ```bash
   scp sing-box-openwrt.json root@192.168.1.1:/etc/sing-box/config.json
   ssh root@192.168.1.1 "/etc/init.d/sing-box restart"
   ```
