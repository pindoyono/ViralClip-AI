package trends

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
