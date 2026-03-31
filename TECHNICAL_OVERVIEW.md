# api-gateway — Teknik Genel Bakış

| Proje | Servis | Dil / Runtime | Son Güncelleme |
|---|---|---|---|
| Tresaurio Kripto Portföy Yönetim Platformu | api-gateway | Go 1.24 | 2026-03-30 |

---

## 1. Servisin Amacı

api-gateway, Tresaurio platformunun tüm client isteklerini karşılayan tek giriş noktasıdır. Gelen HTTP ve WebSocket isteklerini downstream servislere yönlendirir; kimlik doğrulama, hız sınırlama ve hata yönetimi gibi cross-cutting concern'leri merkezi olarak uygular.

Downstream servisler bu gateway'i görmeden doğrudan dış dünyaya açılmaz; tüm trafik buradan geçer.

| Sorumluluk | Açıklama |
|---|---|
| Reverse Proxy | İstekleri market-data ve exchange-service'e yönlendirir |
| JWT Doğrulama | Korumalı endpoint'lerde token doğrular, `X-User-ID` header'ını downstream'e iletir |
| Rate Limiting | IP bazlı istek sınırlaması — saniyede 10, burst 30 |
| Circuit Breaker | Downstream servis art arda 5 hata verirse devreyi açar, 503 döner |
| Health Aggregation | Tüm downstream servislerin durumunu tek endpoint'ten sunar |
| WebSocket Proxy | WebSocket bağlantılarını upstream servise tüneller |

---

## 2. Mimari

```
Client (HTTP / WebSocket)
        │
        ▼
┌───────────────────────────────────────────────────┐
│              api-gateway (port 9000)              │
│                                                   │
│  ┌──────────────┐  ┌───────────┐  ┌────────────┐ │
│  │ Rate Limiter │  │ JWT Auth  │  │  Logger /  │ │
│  │  (global)    │  │(protected │  │  Recoverer │ │
│  │              │  │  routes)  │  │            │ │
│  └──────┬───────┘  └─────┬─────┘  └────────────┘ │
│         │                │                        │
│  ┌──────▼───────────────▼──────────────────────┐ │
│  │              chi Router                      │ │
│  │                                              │ │
│  │  /health           → Health Handler          │ │
│  │  /api/v1/quotes/*  → CircuitBreaker          │ │
│  │  /ws/quotes/*      → WebSocket Proxy         │ │
│  │  /positions/*      → CircuitBreaker + JWT    │ │
│  │  /api/v1/pnl/*     → CircuitBreaker + JWT    │ │
│  │  /api/v1/orders/*  → CircuitBreaker + JWT    │ │
│  │  /ws/positions/*   → WebSocket Proxy + JWT   │ │
│  └──────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────┘
           │                        │
           ▼                        ▼
  ┌─────────────────┐    ┌─────────────────────┐
  │  market-data    │    │   exchange-service  │
  │  (ngrok URL)    │    │   (ngrok URL)       │
  └─────────────────┘    └─────────────────────┘
```

### Middleware Zinciri

Her istek aşağıdaki sırayla middleware'den geçer:

```
Request
   │
   ├─► Logger          (chi — tüm istekleri loglar)
   ├─► Recoverer       (chi — panic'leri yakalar, 500 döner)
   ├─► RateLimiter     (IP bazlı token bucket)
   ├─► [JWTAuth]       (sadece korumalı route group'larında)
   ├─► CircuitBreaker  (market-data veya exchange CB'si)
   └─► Proxy Handler   (reverse proxy / websocket proxy)
```

---

## 3. Bileşen Detayları

### 3.1 Reverse Proxy (`internal/handler/proxy.go`)

Go standart kütüphanesindeki `httputil.ReverseProxy` kullanılarak implement edilmiştir. Her downstream servis için ayrı bir proxy instance'ı oluşturulur.

```go
func NewProxy(target string) http.Handler {
    url, _ := url.Parse(target)
    proxy := httputil.NewSingleHostReverseProxy(url)
    proxy.Director = func(req *http.Request) {
        req.URL.Scheme = url.Scheme
        req.URL.Host   = url.Host
        req.Host       = url.Host
        req.Header.Set("ngrok-skip-browser-warning", "true")
    }
    return proxy
}
```

`Director` fonksiyonu her istekte çalışır ve hedef URL'yi set eder. `ngrok-skip-browser-warning` header'ı, ngrok üzerinden çalışan downstream servislerin browser uyarı sayfası döndürmesini engeller.

---

### 3.2 JWT Doğrulama (`internal/middleware/auth.go`)

Korumalı route group'larına uygulanan middleware. HMAC-SHA algoritmasıyla imzalanmış JWT token'larını doğrular.

```go
token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
    if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
        return nil, jwt.ErrSignatureInvalid
    }
    return []byte(secret), nil
})

// Doğrulama başarılıysa:
userID, _ := claims["id"].(string)
r.Header.Set("X-User-ID", userID)
```

| Adım | Açıklama |
|---|---|
| Header kontrolü | `Authorization: Bearer <token>` formatı zorunlu |
| İmza doğrulama | Yalnızca HMAC yöntemi kabul edilir, diğerleri reddedilir |
| Claim çıkarma | Token içindeki `id` claim'i alınır |
| Header iletimi | `X-User-ID` olarak downstream servise iletilir |
| Context | `userID` değeri request context'ine de yazılır |

Hatalı veya eksik token durumunda `401 Unauthorized` + `{"error":"unauthorized"}` döner.

---

### 3.3 Rate Limiter (`internal/middleware/ratelimit.go`)

Token bucket algoritması kullanılarak implement edilmiştir. Her IP için ayrı bir `rate.Limiter` instance'ı tutulur.

**Mevcut konfigürasyon:** `NewRateLimiter(10, 30)` — saniyede 10 istek, burst 30.

| Özellik | Detay |
|---|---|
| Algoritma | Token bucket (`golang.org/x/time/rate`) |
| Kapsam | IP bazlı, in-memory |
| IP tespiti | Önce `X-Forwarded-For`, yoksa `RemoteAddr` |
| Bellek yönetimi | Background goroutine her dakika çalışır; 3 dakikadır görülmeyen IP'ler silinir |
| Limit aşımı | `429 Too Many Requests` + `{"error":"too many requests"}` |

`cleanupLoop` goroutine'i `sync.Mutex` ile koruma altında çalışır, uzun süreli kullanımda bellek sızıntısını önler.

---

### 3.4 Circuit Breaker (`internal/middleware/circuitbreaker.go`)

Her downstream servis için bağımsız bir circuit breaker instance'ı vardır (`market-data` ve `exchange`).

#### Durum Makinesi

```
          5. hata
Closed ──────────────► Open
  ▲                      │
  │       30 saniye      │
  │    ◄──────────────── │
  │                   Half-Open
  │      başarılı istek  │
  └──────────────────────┘
```

| Durum | Açıklama |
|---|---|
| `Closed` | Normal çalışma — istekler geçer |
| `Open` | Servis çöktü — tüm istekler direkt 503 döner |
| `Half-Open` | Test modu — bir istek geçirilir; başarılıysa `Closed`'a döner |

**Hata tespiti:** Downstream servisin `5xx` HTTP status kodu dönmesi hata sayılır. `4xx` hatalar (client hataları) sayılmaz.

**Open durumunda response:**
```json
{
  "error": "service temporarily unavailable",
  "service": "market-data"
}
```

Tüm durum geçişleri `sync.Mutex` ile thread-safe'tir.

---

### 3.5 Health Aggregation (`internal/handler/health.go`)

`GET /health` endpoint'i downstream servislerin `/health` endpoint'lerini kontrol eder ve birleşik durum döner.

Her servis için **3 saniyelik timeout** uygulanır. Downstream servisler ngrok üzerinden çalıştığından her isteğe `ngrok-skip-browser-warning: true` header'ı eklenir. Herhangi bir servis `down` ise:
- `status` alanı `"degraded"` olur
- HTTP status `503 Service Unavailable` döner

```json
{
  "status": "ok",
  "services": {
    "market-data": { "status": "ok", "latency": "45ms" },
    "exchange":    { "status": "down", "error": "connection refused" }
  }
}
```

---

### 3.6 WebSocket Proxy (`internal/handler/websocket.go`)

`golang.org/x/net/websocket` ile implement edilmiştir. İki yönlü (bidirectional) tünel açar.

```
Client ──── ws/wss ────► api-gateway ──── ws/wss ────► Upstream
               ◄─────────────────────────────────────
```

**Scheme dönüşümü:** HTTP target URL'leri otomatik olarak WebSocket scheme'ine çevrilir:
- `http://` → `ws://`
- `https://` → `wss://`

**Header iletimi:** Yalnızca `Authorization` ve `X-` prefix'li header'lar upstream'e iletilir. JWT middleware tarafından set edilen `X-User-ID` bu sayede downstream'e ulaşır.

**Tampon boyutu:** Her yönde 4096 byte.

> `/ws/positions/*` JWT korumalıdır — WebSocket handshake'ten önce token doğrulanır.

---

## 4. Route Tablosu

| Method | Path | Auth | Downstream | Circuit Breaker |
|---|---|---|---|---|
| `GET` | `/health` | — | market-data + exchange | — |
| `ANY` | `/api/v1/quotes/*` | — | market-data | market-data CB |
| `WS` | `/ws/quotes/*` | — | market-data | — |
| `ANY` | `/positions/*` | JWT | exchange-service | exchange CB |
| `ANY` | `/api/v1/pnl/*` | JWT | exchange-service | exchange CB |
| `ANY` | `/api/v1/orders/*` | JWT | exchange-service | exchange CB |
| `WS` | `/ws/positions/*` | JWT | exchange-service | — |

---

## 5. Konfigürasyon

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `PORT` | `9000` | Gateway'in dinlediği port |
| `JWT_SECRET` | — | JWT doğrulama anahtarı (zorunlu) |
| `MARKET_DATA_URL` | ngrok URL | market-data servisinin adresi |
| `EXCHANGE_URL` | `https://contextured-tora-nontribally.ngrok-free.dev` | exchange-service'in adresi |

`JWT_SECRET` set edilmezse korumalı tüm endpoint'ler `401` döner.

---

## 6. Dosya Yapısı

```
api-gateway/
├── cmd/
│   └── main.go                    # Uygulama giriş noktası — router kurulumu, middleware zinciri, route tanımları
├── config/
│   └── config.go                  # Environment variable okuma ve varsayılan değerler
└── internal/
    ├── handler/
    │   ├── health.go              # GET /health — downstream servis durumu aggregation
    │   ├── proxy.go               # HTTP reverse proxy (httputil.ReverseProxy wrapper)
    │   └── websocket.go           # WebSocket bidirectional tünel
    └── middleware/
        ├── auth.go                # JWT doğrulama — token parse, X-User-ID iletimi
        ├── circuitbreaker.go      # Circuit breaker state machine (Closed / Open / Half-Open)
        ├── circuitbreaker_test.go # Circuit breaker unit testleri
        └── ratelimit.go           # IP bazlı token bucket rate limiter
```

---

## 7. Test Sonuçları

Aşağıdaki testler ngrok üzerinden gerçek ortamda gerçekleştirilmiştir.

### 7.1 Health Aggregation

```bash
curl https://<gateway-ngrok>/health
# → {
#     "status": "ok",
#     "services": {
#       "market-data": { "status": "ok", "latency": "38ms" },
#       "exchange":    { "status": "ok", "latency": "52ms" }
#     }
#   }
```
✓ 3 servis de "ok" döndü — servisler arası bağlantı doğrulandı

### 7.2 JWT Koruması

```bash
# Token olmadan
curl https://<gateway-ngrok>/positions/1
# → {"error":"unauthorized"}  HTTP 401

# Geçerli token ile
curl -H "Authorization: Bearer <token>" https://<gateway-ngrok>/positions/1
# → pozisyon listesi  HTTP 200
```
✓ Korumasız istek reddedildi, geçerli token ile geçti

### 7.3 Rate Limiting

```bash
# 31. istekte (burst 30 aşıldığında)
# → {"error":"too many requests"}  HTTP 429
```
✓ Token bucket davranışı doğrulandı

### 7.4 Circuit Breaker

```bash
# exchange-service kapalıyken
curl -H "Authorization: Bearer <token>" https://<gateway-ngrok>/positions/1
# → {
#     "error": "service temporarily unavailable",
#     "service": "exchange"
#   }  HTTP 503
```
✓ 5 art arda hata sonrası devre açıldı, 30 saniye sonra otomatik kapandı

---

## 8. Bilinen Kısıtlamalar ve Sonraki Adımlar

| # | Kısıtlama | Planlanan Çözüm |
|---|---|---|
| 1 | WebSocket proxy'de circuit breaker yok | WS bağlantıları CB'den geçirilmeli |
| 2 | Health check'ler sıralı çalışıyor | `sync.WaitGroup` ile paralel hale getir |
| 3 | Rate limiter restart'ta sıfırlanır | Redis backed rate limiting |
| 4 | JWT token revocation yok | Token invalidation / blacklist mekanizması |
| 5 | Tek docker-compose yok | Tüm platform tek compose ile ayağa kalksın |
| 6 | Metrics / tracing yok | Prometheus + Jaeger entegrasyonu |

---

*Tresaurio Platform — api-gateway v0.4.0*


*Tresaurio Platform — api-gateway v0.4.0*
