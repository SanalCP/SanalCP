# Kurulum betiğini yayımlama

`.github/workflows/release.yml`, yeni `vX.Y.Z` etiketi geldiğinde `sanalcp.com/kur`
dosyasını o etiketin commit'ine ve arşiv SHA-256'sına sabitler. Sunucu erişimi
onarıldıktan sonra yeni uygulama sürümü çıkarmadan tekrar yayımlamak için:

```bash
gh workflow run release.yml --ref main -f tag=v0.9.61
```

Akış, etiketteki `surum.json` değerini doğrular, `scripts/installer-bootstrap.py`
ile dosyayı üretir ve canlı dosyanın tamamını üretilen dosyayla karşılaştırır.
`main` dalına normal bir push, canlı kurulum sürümünü değiştirmez.

## Sunucu erişimi

GitHub repository secrets: `DEPLOY_HOST`, `DEPLOY_PORT`, `DEPLOY_USER`,
`DEPLOY_KEY`, `DEPLOY_KNOWN_HOSTS`. Sonuncusu, güvenilen SSH bağlantısından
alınan sunucu anahtarını OpenSSH known_hosts biçiminde tutar. Sunucu değişince
bu değerler birlikte güncellenmelidir. Çalışma sırasında `ssh-keyscan` yapılmaz.
Eski `DEPLOY_PATH` artık kullanılmaz; hedef yol root'a ait sunucu ayarındadır.

Sunucudaki `sanalcp-release` hesabının anahtarı `restrict` seçeneğiyle yalnızca
şu zorunlu komutu çalıştırır:

```text
sudo -n /usr/local/libexec/sanalcp-installer-bootstrap publish
```

`/usr/local/libexec/sanalcp-installer-bootstrap`, depodaki
`scripts/installer-bootstrap.py` dosyasının root'a ait, 0755 izinli kopyasıdır.
`/etc/sudoers.d/sanalcp-installer` yalnız bu komuta ve `publish` argümanına
parolasız izin verir. Hesabın home ve authorized_keys dosyaları root'a aittir.

Root'a ait 0600 izinli `/etc/sanalcp-installer-deploy.json` yapılandırmasında
`target` (canlı kur dosyasının tam yolu) ve `backup_dir` (web kökü dışındaki
root'a özel yedek dizini) bulunur. İstemci bu yolları belirleyemez.

Yayımlayıcı yalnız beklenen şablona uyan, commit ve SHA-256 bilgisi içeren
küçük dosyaları kabul eder; gelen betiği çalıştırmaz. Önce mevcut dosyayı
yedekler, sonra dosya sahibi, izinleri ve okuma ACL'sini koruyarak atomik
olarak değiştirir. Sunucudaki yardımcı ve depodaki üretici aynı şablonu
kullanmalıdır; şablon değişikliğinde sunucudaki kopya da güncellenmelidir.

Testler:

```bash
python3 scripts/installer-bootstrap-test.py
```
