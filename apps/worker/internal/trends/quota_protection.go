package trends

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ErrQuotaExceeded indicates the collector reached the current quota budget.
var ErrQuotaExceeded = errors.New("youtube quota budget exceeded")

// RateLimiterService enforces a minimum delay between outbound API calls.
type RateLimiterService struct {
	mu          sync.Mutex
	minInterval time.Duration
	lastRequest time.Time
}

// NewRateLimiterService builds a limiter from requests-per-second.
func NewRateLimiterService(requestsPerSecond int) *RateLimiterService {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 5
	}
	return &RateLimiterService{
		minInterval: time.Second / time.Duration(requestsPerSecond),
	}
}

// Wait blocks until the next request is allowed.
func (s *RateLimiterService) Wait(ctx context.Context) error {
	s.mu.Lock()
	now := time.Now().UTC()
	waitFor := time.Duration(0)
	if !s.lastRequest.IsZero() {
		nextAllowed := s.lastRequest.Add(s.minInterval)
		if nextAllowed.After(now) {
			waitFor = nextAllowed.Sub(now)
		}
	}
	s.mu.Unlock()

	if waitFor > 0 {
		timer := time.NewTimer(waitFor)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	s.mu.Lock()
	s.lastRequest = time.Now().UTC()
	s.mu.Unlock()
	return nil
}

// QuotaMonitorService tracks and guards YouTube quota consumption.
type QuotaMonitorService struct {
	mu           sync.Mutex
	dailyLimit   int
	safetyBuffer int
	usedUnits    int
	exhausted    bool
	resetAt      time.Time
	now          func() time.Time
}

// NewQuotaMonitorService creates a quota monitor.
func NewQuotaMonitorService(dailyLimit, safetyBuffer int) *QuotaMonitorService {
	if dailyLimit <= 0 {
		dailyLimit = 10000
	}
	if safetyBuffer < 0 {
		safetyBuffer = 0
	}
	if safetyBuffer >= dailyLimit {
		safetyBuffer = dailyLimit / 10
	}
	nowFn := func() time.Time { return time.Now().UTC() }
	return &QuotaMonitorService{
		dailyLimit:   dailyLimit,
		safetyBuffer: safetyBuffer,
		resetAt:      nextUTCReset(nowFn()),
		now:          nowFn,
	}
}

// Reserve attempts to consume quota units for the next request.
func (s *QuotaMonitorService) Reserve(units int) error {
	if units <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetIfNeededLocked()
	if s.exhausted {
		return ErrQuotaExceeded
	}
	if s.usedUnits+units > s.dailyLimit-s.safetyBuffer {
		s.exhausted = true
		return ErrQuotaExceeded
	}
	s.usedUnits += units
	return nil
}

// MarkExhausted marks quota as exhausted immediately.
func (s *QuotaMonitorService) MarkExhausted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exhausted = true
}

func (s *QuotaMonitorService) resetIfNeededLocked() {
	if !s.now().Before(s.resetAt) {
		s.usedUnits = 0
		s.exhausted = false
		s.resetAt = nextUTCReset(s.now())
	}
}

func nextUTCReset(now time.Time) time.Time {
	n := now.UTC()
	return time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, time.UTC)
}

// YouTubeCacheService caches API responses to reduce repeated quota usage.
type YouTubeCacheService struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]cacheEntry
}

type cacheEntry struct {
	body      []byte
	expiresAt time.Time
}

// NewYouTubeCacheService creates an in-memory cache.
func NewYouTubeCacheService(ttl time.Duration) *YouTubeCacheService {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &YouTubeCacheService{
		ttl:     ttl,
		entries: make(map[string]cacheEntry),
	}
}

// Get returns cached response for path+params and unmarshals it into target.
func (s *YouTubeCacheService) Get(path string, params url.Values, target interface{}) bool {
	key := cacheKey(path, params)
	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok || time.Now().UTC().After(entry.expiresAt) {
		if ok {
			s.mu.Lock()
			delete(s.entries, key)
			s.mu.Unlock()
		}
		return false
	}
	if err := json.Unmarshal(entry.body, target); err != nil {
		log.Warn().Err(err).Str("cache_key", key).Msg("YouTubeCacheService: failed to unmarshal cached response")
		return false
	}
	return true
}

// Set stores raw JSON response for path+params.
func (s *YouTubeCacheService) Set(path string, params url.Values, body []byte) {
	if len(body) == 0 {
		return
	}
	key := cacheKey(path, params)
	copied := append([]byte(nil), body...)
	s.mu.Lock()
	s.entries[key] = cacheEntry{
		body:      copied,
		expiresAt: time.Now().UTC().Add(s.ttl),
	}
	s.mu.Unlock()
}

func cacheKey(path string, params url.Values) string {
	return path + "?" + params.Encode()
}

func quotaCostForPath(path string) int {
	switch path {
	case "/search":
		return 100
	case "/videos":
		return 1
	case "/channels":
		return 1
	case "/videoCategories":
		return 1
	default:
		return 1
	}
}
