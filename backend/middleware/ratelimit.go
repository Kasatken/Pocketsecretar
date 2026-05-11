package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	count   int
	started time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]bucket
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]bucket),
	}
}

func (l *RateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b := l.buckets[ip]
	if b.started.IsZero() || now.Sub(b.started) > l.window {
		l.buckets[ip] = bucket{count: 1, started: now}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	l.buckets[ip] = b
	return true
}

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !l.Allow(ip) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if strings.Contains(host, ":") {
		parsedHost, _, err := net.SplitHostPort(host)
		if err == nil {
			return parsedHost
		}
	}
	return host
}
