package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// slidingWindowScript kayan pencere sayacını atomik olarak kontrol eder ve artırır.
// İstek geçerliyse 1, limit aşıldıysa 0 döner.
var slidingWindowScript = redis.NewScript(`
local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])

redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local count = redis.call('ZCARD', key)
if count < limit then
    redis.call('ZADD', key, now, now .. '-' .. math.random(1,1000000))
    redis.call('PEXPIRE', key, window + 1000)
    return 1
end
return 0
`)

type RateLimiter struct {
	client   *redis.Client
	windowMs int64
	limit    int
	name     string

	// Redis erişilemez olduğunda devreye giren in-memory yedek (degraded mode)
	fallbackMu      sync.Mutex
	fallbackLimiters map[string]*rate.Limiter
}

func NewRateLimiter(redisAddr string, name string, limit int) *RateLimiter {
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("[rate-limiter:%s] WARNING: Redis unavailable (%v) — starting in degraded mode", name, err)
	}

	return &RateLimiter{
		client:           client,
		windowMs:         1000,
		limit:            limit,
		name:             name,
		fallbackLimiters: make(map[string]*rate.Limiter),
	}
}

// fallbackAllow Redis down olduğunda IP başına in-memory token bucket kullanır.
func (rl *RateLimiter) fallbackAllow(ip string) bool {
	rl.fallbackMu.Lock()
	lim, ok := rl.fallbackLimiters[ip]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(rl.limit), rl.limit)
		rl.fallbackLimiters[ip] = lim
	}
	rl.fallbackMu.Unlock()
	return lim.Allow()
}

func (rl *RateLimiter) allow(ip string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	now := time.Now().UnixMilli()
	key := "rl:" + rl.name + ":" + ip

	result, err := slidingWindowScript.Run(ctx, rl.client, []string{key},
		strconv.FormatInt(now, 10),
		strconv.FormatInt(rl.windowMs, 10),
		rl.limit,
	).Int()
	if err != nil {
		log.Printf("[rate-limiter:%s] redis error, falling back to in-memory: %v", rl.name, err)
		return rl.fallbackAllow(ip)
	}
	return result == 1
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = forwarded
		} else if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}

		if !rl.allow(ip) {
			RecordRateLimitHit()
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
