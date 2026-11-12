package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/apierr"
	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting for the API.
type RateLimiter struct {
	global    *rate.Limiter
	perIP     map[string]*ipLimiter
	mu        sync.RWMutex
	cleanup   *time.Ticker
	ipRate    rate.Limit
	ipBurst   int
	cleanupMu sync.Mutex
}

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter with global and per-IP limits.
// globalRate: requests per second allowed globally
// globalBurst: maximum burst size for global limiter
// ipRate: requests per second allowed per IP
// ipBurst: maximum burst size per IP
func NewRateLimiter(globalRate float64, globalBurst int, ipRate float64, ipBurst int) *RateLimiter {
	rl := &RateLimiter{
		global:  rate.NewLimiter(rate.Limit(globalRate), globalBurst),
		perIP:   make(map[string]*ipLimiter),
		cleanup: time.NewTicker(1 * time.Minute),
		ipRate:  rate.Limit(ipRate),
		ipBurst: ipBurst,
	}

	// Start cleanup goroutine to remove stale IP entries
	go rl.cleanupStaleEntries()

	return rl
}

// getLimiter returns the rate limiter for a given IP address.
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.perIP[ip]
	rl.mu.RUnlock()

	if exists {
		rl.mu.Lock()
		limiter.lastSeen = time.Now()
		rl.mu.Unlock()
		return limiter.limiter
	}

	// Create new limiter for this IP
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := rl.perIP[ip]; exists {
		limiter.lastSeen = time.Now()
		return limiter.limiter
	}

	newLimiter := &ipLimiter{
		limiter:  rate.NewLimiter(rl.ipRate, rl.ipBurst),
		lastSeen: time.Now(),
	}
	rl.perIP[ip] = newLimiter
	return newLimiter.limiter
}

// cleanupStaleEntries removes IP limiters that haven't been used in 3 minutes.
func (rl *RateLimiter) cleanupStaleEntries() {
	for range rl.cleanup.C {
		rl.cleanupMu.Lock()
		rl.mu.Lock()
		for ip, limiter := range rl.perIP {
			if time.Since(limiter.lastSeen) > 3*time.Minute {
				delete(rl.perIP, ip)
			}
		}
		rl.mu.Unlock()
		rl.cleanupMu.Unlock()
	}
}

// Stop stops the cleanup ticker.
func (rl *RateLimiter) Stop() {
	rl.cleanup.Stop()
}

// Limit returns a middleware handler that enforces rate limits.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check global rate limit first
		if !rl.global.Allow() {
			apierr.WriteErrorWithContext(w, r, apierr.RateLimitGlobal())
			return
		}

		// Get client IP from various headers (proxy-aware)
		ip := getClientIP(r)

		// Check per-IP rate limit
		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			apierr.WriteErrorWithContext(w, r, apierr.RateLimitIP())
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts the client IP from the request, checking common proxy headers.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (used by most proxies)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For can contain multiple IPs; take the first one
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header (used by nginx)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}
