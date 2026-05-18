package trends

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrendEngineScore(t *testing.T) {
	engine := NewTrendEngine()
	collectedAt := time.Now().UTC()
	videos := []CollectedVideo{{
		SourcePlatform:  sourcePlatformYouTube,
		ExternalVideoID: "video-1",
		Title:           "Title",
		Views:           1200,
		Likes:           120,
		Comments:        30,
		SubscriberCount: 300,
		PublishedAt:     collectedAt.Add(-2 * time.Hour),
	}}

	scored := engine.Score(videos, map[string]int64{"video-1": 1000}, collectedAt)
	assert.Len(t, scored, 1)
	assert.Equal(t, 600.0, scored[0].ViewVelocity)
	assert.Equal(t, 0.125, scored[0].EngagementRate)
	assert.Equal(t, 4.0, scored[0].OutlierScore)
	assert.Equal(t, 200.0, scored[0].GrowthScore)
	assert.Greater(t, scored[0].ViralScore, 0.0)
}
