package trends

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var defaultSeedQueries = []string{"viral shorts", "trending shorts", "viral clips"}

// Collector abstracts the upstream trend source.
type Collector interface {
	Collect(ctx context.Context, queries []string) ([]CollectedVideo, error)
}

// TrendCollectorWorker periodically fetches and stores viral opportunities.
type TrendCollectorWorker struct {
	db        *gorm.DB
	collector Collector
	engine    *TrendEngine
	now       func() time.Time
}

// ViralOpportunity is the worker-side DB representation.
type ViralOpportunity struct {
	ID              string `gorm:"primaryKey"`
	SourcePlatform  string `gorm:"uniqueIndex:idx_viral_opportunity_source,priority:1"`
	ExternalVideoID string `gorm:"uniqueIndex:idx_viral_opportunity_source,priority:2"`
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
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (ViralOpportunity) TableName() string { return "viral_opportunities" }

func (v *ViralOpportunity) BeforeCreate(tx *gorm.DB) error {
	if v.ID == "" {
		v.ID = newUUID()
	}
	return nil
}

// ContentProfile is used to derive seed queries.
type ContentProfile struct {
	ID       string
	Niche    string
	Keywords string
}

func (ContentProfile) TableName() string { return "content_profiles" }

// NewTrendCollectorWorker creates a trend collector worker.
func NewTrendCollectorWorker(db *gorm.DB, collector Collector, engine *TrendEngine) *TrendCollectorWorker {
	return &TrendCollectorWorker{db: db, collector: collector, engine: engine, now: func() time.Time { return time.Now().UTC() }}
}

// RunOnce executes a single collection cycle.
func (w *TrendCollectorWorker) RunOnce(ctx context.Context) error {
	queries, err := w.buildQueries(ctx)
	if err != nil {
		return err
	}
	videos, err := w.collector.Collect(ctx, queries)
	if err != nil {
		if errors.Is(err, ErrCollectorDisabled) {
			log.Warn().Msg("TrendCollectorWorker skipped because YouTube collector is not configured")
			return nil
		}
		return err
	}
	if len(videos) == 0 {
		log.Info().Strs("queries", queries).Msg("TrendCollectorWorker found no YouTube candidates")
		return nil
	}

	previousViews, err := w.loadPreviousViews(ctx, videos)
	if err != nil {
		return err
	}
	scored := w.engine.Score(videos, previousViews, w.now())
	if err := w.upsert(ctx, scored); err != nil {
		return err
	}

	log.Info().Int("videos", len(scored)).Int("queries", len(queries)).Msg("TrendCollectorWorker collected viral opportunities")
	return nil
}

func (w *TrendCollectorWorker) buildQueries(ctx context.Context) ([]string, error) {
	queries := append([]string(nil), defaultSeedQueries...)
	var profiles []ContentProfile
	if err := w.db.WithContext(ctx).Limit(50).Find(&profiles).Error; err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if niche := strings.TrimSpace(profile.Niche); niche != "" {
			queries = append(queries, niche)
		}
		for _, keyword := range strings.Split(profile.Keywords, ",") {
			if keyword = strings.TrimSpace(keyword); keyword != "" {
				queries = append(queries, keyword)
			}
		}
	}
	return unique(queries), nil
}

func (w *TrendCollectorWorker) loadPreviousViews(ctx context.Context, videos []CollectedVideo) (map[string]int64, error) {
	ids := make([]string, 0, len(videos))
	for _, video := range videos {
		ids = append(ids, video.ExternalVideoID)
	}
	var existing []ViralOpportunity
	if err := w.db.WithContext(ctx).Where("external_video_id IN ?", ids).Find(&existing).Error; err != nil {
		return nil, err
	}
	previous := make(map[string]int64, len(existing))
	for _, item := range existing {
		previous[item.ExternalVideoID] = item.Views
	}
	return previous, nil
}

func (w *TrendCollectorWorker) upsert(ctx context.Context, scored []ScoredOpportunity) error {
	records := make([]ViralOpportunity, 0, len(scored))
	for _, item := range scored {
		records = append(records, ViralOpportunity{
			SourcePlatform:  item.SourcePlatform,
			ExternalVideoID: item.ExternalVideoID,
			ChannelID:       item.ChannelID,
			Title:           item.Title,
			Category:        item.Category,
			SourceQuery:     item.SourceQuery,
			Views:           item.Views,
			PreviousViews:   item.PreviousViews,
			Likes:           item.Likes,
			Comments:        item.Comments,
			SubscriberCount: item.SubscriberCount,
			PublishedAt:     item.PublishedAt,
			LastCollectedAt: item.LastCollectedAt,
			ViewVelocity:    item.ViewVelocity,
			EngagementRate:  item.EngagementRate,
			OutlierScore:    item.OutlierScore,
			GrowthScore:     item.GrowthScore,
			ViralScore:      item.ViralScore,
		})
	}

	return w.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_platform"}, {Name: "external_video_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"channel_id",
			"title",
			"category",
			"source_query",
			"views",
			"previous_views",
			"likes",
			"comments",
			"subscriber_count",
			"published_at",
			"last_collected_at",
			"view_velocity",
			"engagement_rate",
			"outlier_score",
			"growth_score",
			"viral_score",
			"updated_at",
		}),
	}).Create(&records).Error
}

func newUUID() string {
	return uuid.NewString()
}
