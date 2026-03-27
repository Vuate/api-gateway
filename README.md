# API Gateway — Treasurio

Treasurio projesinin API gateway servisi. Go ile yazılmış, tüm client isteklerini downstream servislere yönlendirir.

## Genel Yapı

```
Client
  ↓
API Gateway (port 9000)
  ├── /api/v1/quotes/*   → market-data servisi
  ├── /ws/quotes/*       → market-data WebSocket
  ├── /positions/*       → exchange-api servisi (JWT zorunlu)
  ├── /api/v1/pnl/*      → exchange-api servisi (JWT zorunlu)
  ├── /api/v1/orders/*   → exchange-api servisi (JWT zorunlu)
  ├── /ws/positions/*    → exchange-api WebSocket (JWT zorunlu)
  └── /health            → tüm servislerin durumu
```

## Özellikler

### Rate Limiting
Saniyede 10 istek, 30 burst. IP bazlı, in-memory tutulur.

### JWT Auth
`/positions`, `/api/v1/pnl`, `/api/v1/orders` endpoint'leri JWT token gerektirir.
Header: `Authorization: Bearer <token>`
Token içindeki `id` claim'i `X-User-ID` header'ı olarak downstream servise iletilir.

### Circuit Breaker
Downstream servis art arda 5 hata verirse devre açılır. Servise istek gönderilmez, direkt `503` döner.
30 saniye sonra otomatik test eder, servis düzeldiyse kapanır.

```json
{
  "error": "service temporarily unavailable",
  "service": "market-data"
}
```

### Health Aggregation
`GET /health` — tüm downstream servislerin durumunu tek endpoint'ten döner.

```json
{
  "status": "ok",
  "services": {
    "market-data": { "status": "ok", "latency": "45ms" },
    "exchange":    { "status": "down", "error": "connection refused" }
  }
}
```

Herhangi bir servis down ise `status: "degraded"` döner ve HTTP 503 verilir.

### WebSocket Proxy
`/ws/quotes/*` ve `/ws/positions/*` path'leri WebSocket bağlantılarını upstream servise tüneller.
`Authorization` ve `X-` prefix'li header'lar upstream'e iletilir.

> `/ws/positions/*` JWT korumalıdır, bağlantı kurulmadan önce token doğrulanır.

## Konfigürasyon

Environment variable ile yapılandırılır:

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `PORT` | `9000` | Gateway'in dinlediği port |
| `JWT_SECRET` | — | JWT doğrulama anahtarı |
| `MARKET_DATA_URL` | ngrok URL | market-data servisinin adresi |
| `EXCHANGE_URL` | `http://host.docker.internal:8081` | exchange-api servisinin adresi |

## Dosya Yapısı

```
api-gateway/
├── cmd/
│   └── main.go                         # Uygulama giriş noktası, route tanımları
├── config/
│   └── config.go                       # Environment variable yönetimi
└── internal/
    ├── handler/
    │   ├── health.go                   # Health aggregation endpoint
    │   ├── proxy.go                    # HTTP reverse proxy
    │   └── websocket.go                # WebSocket proxy
    └── middleware/
        ├── auth.go                     # JWT doğrulama
        ├── circuitbreaker.go           # Circuit breaker implementasyonu
        ├── circuitbreaker_test.go      # Circuit breaker testleri
        └── ratelimit.go                # Rate limiting
```

## Çalıştırma

```bash
go run ./cmd/main.go
```

Test:
```bash
go test ./...
```
