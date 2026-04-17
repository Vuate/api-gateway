package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowScript atomically checks and increments a sliding window counter.
// Returns 1 if request is allowed, 0 if rate limit exceeded.
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
		log.Printf("WARNING: Redis connection failed (%v) — rate limiter will fail open", err)
	}

	return &RateLimiter{
		client:   client,
		windowMs: 1000,
		limit:    limit,
		name:     name,
	}
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
		// Fail open: Redis unavailable, let the request through
		log.Printf("rate limiter redis error: %v", err)
		return true
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
