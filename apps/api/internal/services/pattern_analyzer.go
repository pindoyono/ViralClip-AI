package services

import (
	"context"
	"math"
	"sort"
)

// PatternAnalyzer aggregates hook-level performance into actionable patterns.
type PatternAnalyzer struct {
	tracker *HookPerformanceTracker
}

// NewPatternAnalyzer constructs a pattern analyzer.
func NewPatternAnalyzer(tracker *HookPerformanceTracker) *PatternAnalyzer {
	return &PatternAnalyzer{tracker: tracker}
}

// Analyze computes average CPS and relative improvement per hook type.
func (a *PatternAnalyzer) Analyze(ctx context.Context, userID string, platform string) ([]HookPattern, error) {
	records, err := a.tracker.Collect(ctx, userID, platform)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []HookPattern{}, nil
	}

	type agg struct {
		totalCPS float64
		totalV   float64
		count    int
	}
	byHook := make(map[string]*agg)
	for _, r := range records {
		if _, ok := byHook[r.HookType]; !ok {
			byHook[r.HookType] = &agg{}
		}
		byHook[r.HookType].totalCPS += r.CPS
		byHook[r.HookType].totalV += float64(r.Views)
		byHook[r.HookType].count++
	}

	totalBaseline := 0.0
	for _, a := range byHook {
		totalBaseline += a.totalCPS / float64(a.count)
	}
	baseline := totalBaseline / float64(len(byHook))

	patterns := make([]HookPattern, 0, len(byHook))
	for hookType, a := range byHook {
		avgCPS := a.totalCPS / float64(a.count)
		avgViews := a.totalV / float64(a.count)
		improvement := 0.0
		if baseline > 0 {
			improvement = math.Round(((avgCPS-baseline)/baseline)*1000) / 10
		}
		patterns = append(patterns, HookPattern{
			HookType:    hookType,
			AvgCPS:      math.Round(avgCPS*100) / 100,
			ClipCount:   a.count,
			AvgViews:    math.Round(avgViews),
			Improvement: improvement,
		})
	}

	sort.Slice(patterns, func(i, j int) bool { return patterns[i].AvgCPS > patterns[j].AvgCPS })
	return patterns, nil
}
