package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// CPS weight constants (must sum to 1.0)
const (
	cpsWeightWatchTime      = 0.30
	cpsWeightRetention      = 0.25
	cpsWeightEngagement     = 0.20
	cpsWeightCTR            = 0.15
	cpsWeightSubscriberGain = 0.10
)

// CPSMetrics holds the raw metrics used to compute CPS.
type CPSMetrics struct {
	Views          int64
	WatchTime      float64 // avg watch time in seconds
	Duration       float64 // clip duration in seconds (for retention calc)
	Likes          int64
	Comments       int64
	Shares         int64
	Saves          int64
	CTR            float64 // click-through rate (0–1)
	SubscriberGain int64
}

// ComputeCPS calculates the Content Performance Score (0–100) using the
// weighted formula defined by Task 3:
//
//	CPS = WatchTime×30% + Retention×25% + Engagement×20% + CTR×15% + SubscriberGain×10%
//
// Each component is first normalised to a 0–1 scalar before weighting.
func ComputeCPS(m CPSMetrics) float64 {
	// Watch-time component: normalise against 60 s cap.
	watchTimeCapped := math.Min(m.WatchTime, 60)
	watchTimeNorm := watchTimeCapped / 60

	// Retention component: WatchTime / Duration (capped at 1).
	retentionNorm := 0.0
	if m.Duration > 0 {
		retentionNorm = math.Min(m.WatchTime/m.Duration, 1)
	}

	// Engagement component: (likes+comments+shares+saves) / views (capped at 1).
	engagementNorm := 0.0
	if m.Views > 0 {
		raw := float64(m.Likes+m.Comments+m.Shares+m.Saves) / float64(m.Views)
		engagementNorm = math.Min(raw, 1)
	}

	// CTR component: already 0–1; cap at 1 for safety.
	ctrNorm := math.Min(m.CTR, 1)

	// SubscriberGain component: normalise against 1000 cap.
	subGainNorm := math.Min(float64(m.SubscriberGain)/1000, 1)

	raw := watchTimeNorm*cpsWeightWatchTime +
		retentionNorm*cpsWeightRetention +
		engagementNorm*cpsWeightEngagement +
		ctrNorm*cpsWeightCTR +
		subGainNorm*cpsWeightSubscriberGain

	return math.Round(raw*10000) / 100 // 0–100, 2 decimal places
}

// ClipWithCPS associates a clip with its computed CPS and the aggregated metrics.
type ClipWithCPS struct {
	ClipID     uuid.UUID
	Title      string
	Platform   string
	CPS        float64
	Views      int64
	Likes      int64
	Comments   int64
	WatchTime  float64
	Duration   float64
	ViralScore float64
}

// HookPattern summarises performance for a specific hook type.
type HookPattern struct {
	HookType    string
	AvgCPS      float64
	ClipCount   int
	AvgViews    float64
	Improvement float64 // percentage better than baseline
}

// Recommendation is a human-readable learning insight for a content profile.
type Recommendation struct {
	ProfileName string
	Platform    string
	Insight     string
	Confidence  float64 // 0–1
}

// LearningEngine provides CPS calculation, pattern analysis, and
// content-level recommendations from aggregated clip analytics.
type LearningEngine struct {
	db              *gorm.DB
	patternAnalyzer *PatternAnalyzer
}

// NewLearningEngine constructs a new LearningEngine.
func NewLearningEngine(db *gorm.DB) *LearningEngine {
	tracker := NewHookPerformanceTracker(db)
	return &LearningEngine{
		db:              db,
		patternAnalyzer: NewPatternAnalyzer(tracker),
	}
}

// TopClips returns the highest-CPS clips for a user, optionally filtered by platform.
func (e *LearningEngine) TopClips(ctx context.Context, userID string, platform string, limit int) ([]ClipWithCPS, error) {
	all, err := e.rankedClips(ctx, userID, platform)
	if err != nil {
		return nil, err
	}
	// sort descending by CPS
	sort.Slice(all, func(i, j int) bool { return all[i].CPS > all[j].CPS })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// WorstClips returns the lowest-CPS clips for a user, optionally filtered by platform.
func (e *LearningEngine) WorstClips(ctx context.Context, userID string, platform string, limit int) ([]ClipWithCPS, error) {
	all, err := e.rankedClips(ctx, userID, platform)
	if err != nil {
		return nil, err
	}
	// sort ascending by CPS
	sort.Slice(all, func(i, j int) bool { return all[i].CPS < all[j].CPS })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// HookPatterns returns aggregated CPS performance grouped by hook type.
func (e *LearningEngine) HookPatterns(ctx context.Context, userID string, platform string) ([]HookPattern, error) {
	return e.patternAnalyzer.Analyze(ctx, userID, platform)
}

// Recommendations produces learning insights per content profile.
func (e *LearningEngine) Recommendations(ctx context.Context, userID string) ([]Recommendation, error) {
	// Fetch user content profiles.
	var profiles []models.ContentProfile
	if err := e.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&profiles).Error; err != nil {
		return nil, err
	}

	if len(profiles) == 0 {
		return []Recommendation{}, nil
	}

	patterns, err := e.HookPatterns(ctx, userID, "")
	if err != nil {
		return nil, err
	}

	// Build per-platform clip CPS vectors.
	type platformKey struct{ platform string }
	type durationGroup struct {
		shortCPS []float64 // < 30 s
		longCPS  []float64 // >= 30 s
	}
	durationCPS := map[string]*durationGroup{}

	clips, err := e.rankedClips(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	for _, c := range clips {
		key := c.Platform
		if _, ok := durationCPS[key]; !ok {
			durationCPS[key] = &durationGroup{}
		}
		if c.Duration > 0 && c.Duration < 30 {
			durationCPS[key].shortCPS = append(durationCPS[key].shortCPS, c.CPS)
		} else {
			durationCPS[key].longCPS = append(durationCPS[key].longCPS, c.CPS)
		}
	}

	var recs []Recommendation
	for _, profile := range profiles {
		// Hook-type recommendation.
		if len(patterns) > 0 {
			best := patterns[0]
			if best.Improvement > 0 {
				recs = append(recs, Recommendation{
					ProfileName: profile.Name,
					Platform:    profile.Platform,
					Insight: fmt.Sprintf(
						"%s hooks perform %.0f%% better for %s profile",
						strings.Title(best.HookType), best.Improvement, profile.Name,
					),
					Confidence: math.Min(float64(best.ClipCount)/10, 0.95),
				})
			}
		}

		// Duration recommendation.
		if dg, ok := durationCPS[profile.Platform]; ok {
			shortMean := mean(dg.shortCPS)
			longMean := mean(dg.longCPS)
			if shortMean > 0 && longMean > 0 {
				diff := math.Round(((shortMean-longMean)/longMean)*1000) / 10
				if diff > 0 {
					recs = append(recs, Recommendation{
						ProfileName: profile.Name,
						Platform:    profile.Platform,
						Insight: fmt.Sprintf(
							"Shorter clips perform %.0f%% better for %s profile",
							diff, profile.Name,
						),
						Confidence: 0.7,
					})
				}
			}
		}
	}

	if len(recs) == 0 {
		recs = []Recommendation{}
	}
	return recs, nil
}

// rankedClips loads all clip analytics for a user, computes CPS per clip+platform,
// and returns a flat list.
func (e *LearningEngine) rankedClips(ctx context.Context, userID string, platform string) ([]ClipWithCPS, error) {
	type row struct {
		ClipID         string
		Title          string
		Platform       string
		Views          int64
		Likes          int64
		Comments       int64
		Shares         int64
		Saves          int64
		WatchTime      float64
		Duration       float64
		ViralScore     float64
		CTR            float64
		SubscriberGain int64
	}

	q := e.db.WithContext(ctx).
		Table("clip_analytics ca").
		Select(`ca.clip_id,
			c.title,
			ca.platform,
			SUM(ca.views) AS views,
			SUM(ca.likes) AS likes,
			SUM(ca.comments) AS comments,
			SUM(ca.shares) AS shares,
			SUM(ca.saves) AS saves,
			AVG(ca.watch_time) AS watch_time,
			MAX(c.duration) AS duration,
			MAX(c.viral_score) AS viral_score,
			AVG(ca.ctr) AS ctr,
			SUM(ca.subscriber_gain) AS subscriber_gain`).
		Joins("JOIN clips c ON c.id = ca.clip_id").
		Where("c.user_id = ? AND c.deleted_at IS NULL", userID).
		Group("ca.clip_id, c.title, ca.platform")

	if platform != "" {
		q = q.Where("ca.platform = ?", platform)
	}

	var rows []row
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]ClipWithCPS, 0, len(rows))
	for _, r := range rows {
		m := CPSMetrics{
			Views:          r.Views,
			WatchTime:      r.WatchTime,
			Duration:       r.Duration,
			Likes:          r.Likes,
			Comments:       r.Comments,
			Shares:         r.Shares,
			Saves:          r.Saves,
			CTR:            r.CTR,
			SubscriberGain: r.SubscriberGain,
		}
		cid, _ := uuid.Parse(r.ClipID)
		out = append(out, ClipWithCPS{
			ClipID:     cid,
			Title:      r.Title,
			Platform:   r.Platform,
			CPS:        ComputeCPS(m),
			Views:      r.Views,
			Likes:      r.Likes,
			Comments:   r.Comments,
			WatchTime:  r.WatchTime,
			Duration:   r.Duration,
			ViralScore: r.ViralScore,
		})
	}
	return out, nil
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
