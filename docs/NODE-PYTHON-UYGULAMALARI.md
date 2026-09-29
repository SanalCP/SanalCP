# Reverse proxy sitelerinde Node.js / Python uygulamaları

Önce **Reverse Proxy** türünde bir site açın. Web Sunucusu ekranında hedef
protokolünü `http`, hedef adresini `127.0.0.1` olarak tutup yerel portu seçin;
aynı ekrandaki **Node.js / Python Uygulaması** kartında giriş
dosyasını kaydedin, bağımlılıkları kurun ve başlatın. Giriş dosyası site kullanıcısının
`public_html` dizini altında olmalıdır. Yorumlayıcı sunucuda kurulu değilse
kart bunu gösterir.

Servis site kullanıcısıyla ve sitenin systemd kaynak slice'ında çalışır.
Uygulama `HOST=127.0.0.1` ve `PORT` ortam değişkenlerini kullanarak yerel
portu dinlemelidir. Panel, servis başlangıcından sonra bu portun açıldığını
kontrol eder. Hata halinde önceki servis tanımı geri yüklenir.

Panel, sunucuda kurulu Node.js/Python yorumlayıcılarını listeler. Seçilen
yorumlayıcının yolu uygulama ayarında saklanır. Yeni sürüm kurmak sunucu
yöneticisinin sorumluluğundadır; panel sürüm indirmez.

Node.js için `public_html/package.json` ve `package-lock.json` yükleyip
**Bağımlılıkları kur** düğmesine basın. Panel, seçilen Node.js kurulumunun
`npm` komutuyla `npm ci --omit=dev` çalıştırır. Python için
`public_html/requirements.txt` yükleyin. Panel, tenant'ın ev dizininde
uygulamaya ve seçili yorumlayıcıya özel sanal ortam açar, ardından o ortamın
`pip` komutuyla gereksinimleri kurar. Her iki işlem de site kullanıcısının
kimliğiyle çalışır. Kurulumdan sonra uygulamayı yeniden başlatın.

Sunucudaki Python kurulumunda `venv` ve `pip` modülleri hazır olmalıdır.

## Git dağıtımı

Uygulama çalışırken Git hedef dizini `public_html` ise **Pull** ve imzalı
GitHub webhook'u yönetilen dağıtım yapar. Başlamadan önce mevcut uygulamanın
sağlık yolundan HTTP 2xx dönmesi gerekir. Panel yeni commit'i tenant'ın
`.sanalcp/releases/<site-id>/` dizininde ayrı bir Git worktree olarak açar.
Node.js için bu dizinde lockfile üzerinden `npm ci`, varsa `npm run build` ve
üretim bağımlılıkları için `npm prune` çalışır. Python için bu sürüme özel
`.venv` içinde gereksinimler kurulur. Hazırlık başarılıysa servis yeni sürüme
yönlendirilir ve sağlık yolu HTTP 2xx dönene kadar kontrol edilir. Hata halinde
servis önceki sürüme döner; önceki sürümün dosyaları ve bağımlılıkları
hazırlık sırasında değiştirilmez.
Panel geçiş sırasında yeniden başlarsa açılışta kayıtlı sürümü servis dosyasına
yeniden uygular ve sağlık kontrolünü çalıştırır.

İlk klonlamayı uygulamayı başlatmadan önce yapın. Çalışan uygulamada yeniden
klonlama engellenir; sonraki güncellemelerde Pull kullanın. İlk dağıtımdan
sonra aktif uygulama `.sanalcp/releases/` altından çalışır; `public_html`
Git kaynak deposu olarak kalır ve oradaki dosyaları doğrudan düzenlemek aktif
uygulamayı değiştirmez. Dağıtım sırasında servis yeniden başladığı için kısa
bir kesinti olabilir; sıfır kesintili geçiş garantisi yoktur. Uygulamanın
paket betikleri tenant kimliğiyle çalışır ve aynı tenant'ın paylaşılan veri
dizinlerine yazabilir; bu veriler sürüm geri almasından bağımsızdır.
Git sürümü etkinken paneldeki elle bağımlılık kurma işlemi kapatılır; bağımlılık
değişikliklerini yeni commit ile dağıtın.
Canlı sunucuda kabul testi yapılana kadar üretim uygulamasında kullanmadan önce
bir test sitesinde doğrulayın.

## Bağımlılıksız Node.js örneği

`public_html/server.js`:

```js
const http = require('node:http')
const host = process.env.HOST || '127.0.0.1'
const port = Number(process.env.PORT || 3000)
http.createServer((_, response) => {
  response.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' })
  response.end('SanalCP Node.js uygulaması çalışıyor\n')
}).listen(port, host)
```

## Bağımlılıksız Python örneği

`public_html/app.py`:

```python
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = 'SanalCP Python uygulaması çalışıyor\n'.encode()
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain; charset=utf-8')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

HTTPServer((os.getenv('HOST', '127.0.0.1'), int(os.getenv('PORT', '3000'))), Handler).serve_forever()
```

Bu örnekler yalnızca çalışma akışını sınamak içindir. Gerçek uygulamada
bağımlılık, veri ve dağıtım planını ayrıca kurun.
