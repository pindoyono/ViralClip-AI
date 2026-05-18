package trends

import (
	"errors"
	"time"
)

var ErrCollectorDisabled = errors.New("youtube collector is disabled")

// TrendEngine computes opportunity metrics from collected YouTube videos.
type TrendEngine struct{}

// ScoredOpportunity is the persisted representation of a collected trend.
type ScoredOpportunity struct {
	SourcePlatform  string
	ExternalVideoID string
	ChannelID       string
	Title           string
	Category        string
	SourceQuery     string
	Views           int64
	PreviousViews   int64
	Likes           int64
	Comments        int64
	SubscriberCount int64
	PublishedAt     time.Time
	LastCollectedAt time.Time
	ViewVelocity    float64
	EngagementRate  float64
	OutlierScore    float64
	GrowthScore     float64
	ViralScore      float64
}

// NewTrendEngine creates a new TrendEngine.
func NewTrendEngine() *TrendEngine { return &TrendEngine{} }

// Score enriches collected videos with derived trend metrics.
func (e *TrendEngine) Score(videos []CollectedVideo, previousViews map[string]int64, collectedAt time.Time) []ScoredOpportunity {
	scored := make([]ScoredOpportunity, 0, len(videos))
	for _, video := range videos {
		hoursSincePublish := collectedAt.Sub(video.PublishedAt).Hours()
		if hoursSincePublish < 1 {
			hoursSincePublish = 1
		}
		previous := previousViews[video.ExternalVideoID]
		viewVelocity := float64(video.Views) / hoursSincePublish
		engagementRate := 0.0
		if video.Views > 0 {
			engagementRate = float64(video.Likes+video.Comments) / float64(video.Views)
		}
		outlierScore := float64(video.Views)
		if video.SubscriberCount > 0 {
			outlierScore = float64(video.Views) / float64(video.SubscriberCount)
		}
		growthScore := float64(video.Views - previous)
		viralScore := (viewVelocity * 0.35) + (outlierScore * 0.25) + (engagementRate * 0.20) + (growthScore * 0.20)
		scored = append(scored, ScoredOpportunity{
			SourcePlatform:  video.SourcePlatform,
			ExternalVideoID: video.ExternalVideoID,
			ChannelID:       video.ChannelID,
			Title:           video.Title,
			Category:        video.Category,
			SourceQuery:     video.SourceQuery,
			Views:           video.Views,
			PreviousViews:   previous,
			Likes:           video.Likes,
			Comments:        video.Comments,
			SubscriberCount: video.SubscriberCount,
			PublishedAt:     video.PublishedAt,
			LastCollectedAt: collectedAt,
			ViewVelocity:    roundMetric(viewVelocity),
			EngagementRate:  roundMetric(engagementRate),
			OutlierScore:    roundMetric(outlierScore),
			GrowthScore:     roundMetric(growthScore),
			ViralScore:      roundMetric(viralScore),
		})
	}
	return scored
}
