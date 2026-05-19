package trends

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYouTubeCollectorCollect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videoCategories":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": "20", "snippet": map[string]interface{}{"title": "Gaming"}}}})
		case "/search":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": map[string]interface{}{"videoId": "video-1"}}}})
		case "/videos":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{
				"id":         "video-1",
				"snippet":    map[string]interface{}{"title": "Gaming clip", "categoryId": "20", "channelId": "channel-1", "publishedAt": time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)},
				"statistics": map[string]interface{}{"viewCount": "1000", "likeCount": "80", "commentCount": "20"},
			}}})
		case "/channels":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": "channel-1", "statistics": map[string]interface{}{"subscriberCount": "250"}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	collector := NewYouTubeCollector("test-key", "US", 5, 24*time.Hour, server.Client()).WithBaseURL(server.URL)
	videos, err := collector.Collect(t.Context(), []string{"gaming"})
	require.NoError(t, err)
	require.Len(t, videos, 1)
	assert.Equal(t, int64(1000), videos[0].Views)
	assert.Equal(t, int64(250), videos[0].SubscriberCount)
	assert.Equal(t, "Gaming", videos[0].Category)
	assert.Equal(t, "gaming", videos[0].SourceQuery)
}

func TestYouTubeCollectorCollect_UsesCache(t *testing.T) {
	var categoriesCalls int32
	var searchCalls int32
	var videosCalls int32
	var channelsCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videoCategories":
			atomic.AddInt32(&categoriesCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": "20", "snippet": map[string]interface{}{"title": "Gaming"}}}})
		case "/search":
			atomic.AddInt32(&searchCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": map[string]interface{}{"videoId": "video-1"}}}})
		case "/videos":
			atomic.AddInt32(&videosCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{
				"id":         "video-1",
				"snippet":    map[string]interface{}{"title": "Gaming clip", "categoryId": "20", "channelId": "channel-1", "publishedAt": time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)},
				"statistics": map[string]interface{}{"viewCount": "1000", "likeCount": "80", "commentCount": "20"},
			}}})
		case "/channels":
			atomic.AddInt32(&channelsCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": "channel-1", "statistics": map[string]interface{}{"subscriberCount": "250"}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	collector := NewYouTubeCollector("test-key", "US", 5, 24*time.Hour, server.Client()).
		WithBaseURL(server.URL).
		WithCache(NewYouTubeCacheService(time.Hour))

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	mu := sync.Mutex{}
	collector.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}

	_, err := collector.Collect(t.Context(), []string{"gaming"})
	require.NoError(t, err)
	_, err = collector.Collect(t.Context(), []string{"gaming"})
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&categoriesCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&searchCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&videosCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&channelsCalls))
}

func TestYouTubeCollectorCollect_QuotaExhaustedGraceful(t *testing.T) {
	var categoriesCalls int32
	var searchCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videoCategories":
			atomic.AddInt32(&categoriesCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": "20", "snippet": map[string]interface{}{"title": "Gaming"}}}})
		case "/search":
			atomic.AddInt32(&searchCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []map[string]interface{}{{"id": map[string]interface{}{"videoId": "video-1"}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	collector := NewYouTubeCollector("test-key", "US", 5, 24*time.Hour, server.Client()).
		WithBaseURL(server.URL).
		WithQuotaMonitor(NewQuotaMonitorService(50, 0)) // /search needs 100 units -> should be blocked

	videos, err := collector.Collect(t.Context(), []string{"gaming"})
	require.NoError(t, err)
	assert.Len(t, videos, 0)
	assert.Equal(t, int32(1), atomic.LoadInt32(&categoriesCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&searchCalls))
}
