package services

import (
	"context"
	"math"

	"gorm.io/gorm"
)

// HookPerformanceRecord contains collected per-hook performance metrics.
type HookPerformanceRecord struct {
	HookType       string
	ClipID         string
	Views          int64
	WatchTime      float64
	Retention      float64
	Likes          int64
	Comments       int64
	CTR            float64
	SubscriberGain int64
	CPS            float64
}

// HookPerformanceTracker collects hook-level performance metrics.
type HookPerformanceTracker struct {
	db *gorm.DB
}

// NewHookPerformanceTracker constructs a hook performance tracker.
func NewHookPerformanceTracker(db *gorm.DB) *HookPerformanceTracker {
	return &HookPerformanceTracker{db: db}
}

// Collect returns per-hook records with all required learning-loop metrics.
func (t *HookPerformanceTracker) Collect(ctx context.Context, userID string, platform string) ([]HookPerformanceRecord, error) {
	type row struct {
		HookType       string
		ClipID         string
		Views          int64
		WatchTime      float64
		Duration       float64
		Likes          int64
		Comments       int64
		Shares         int64
		Saves          int64
		CTR            float64
		SubscriberGain int64
	}

	query := t.db.WithContext(ctx).
		Table("hook_detections hd").
		Select(`hd.hook_type AS hook_type,
			c.id AS clip_id,
			SUM(ca.views) AS views,
			AVG(ca.watch_time) AS watch_time,
			MAX(c.duration) AS duration,
			SUM(ca.likes) AS likes,
			SUM(ca.comments) AS comments,
			SUM(ca.shares) AS shares,
			SUM(ca.saves) AS saves,
			AVG(ca.ctr) AS ctr,
			SUM(ca.subscriber_gain) AS subscriber_gain`).
		Joins("JOIN videos v ON v.id = hd.video_id").
		Joins("JOIN clips c ON c.video_id = v.id").
		Joins("JOIN clip_analytics ca ON ca.clip_id = c.id").
		Where("v.user_id = ?", userID).
		Group("hd.hook_type, c.id")

	if platform != "" {
		query = query.Where("ca.platform = ?", platform)
	}

	var rows []row
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	records := make([]HookPerformanceRecord, 0, len(rows))
	for _, r := range rows {
		retention := 0.0
		if r.Duration > 0 {
			retention = math.Min(r.WatchTime/r.Duration, 1)
		}
		metrics := CPSMetrics{
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
		records = append(records, HookPerformanceRecord{
			HookType:       r.HookType,
			ClipID:         r.ClipID,
			Views:          r.Views,
			WatchTime:      r.WatchTime,
			Retention:      retention,
			Likes:          r.Likes,
			Comments:       r.Comments,
			CTR:            r.CTR,
			SubscriberGain: r.SubscriberGain,
			CPS:            ComputeCPS(metrics),
		})
	}

	return records, nil
}
