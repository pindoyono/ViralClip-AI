package services

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

const (
	trendingWindow       = 72 * time.Hour
	recommendationWindow = 7 * 24 * time.Hour
	defaultFitScore      = 15
)

// ViralOpportunityFilters controls list queries.
type ViralOpportunityFilters struct {
	Page     int
	Limit    int
	Category string
	Query    string
}

// ViralOpportunityService provides read access to collected opportunities.
type ViralOpportunityService struct {
	db                   *gorm.DB
	recommendationEngine *RecommendationEngine
}

// NewViralOpportunityService constructs a viral opportunity service.
func NewViralOpportunityService(db *gorm.DB, recommendationEngine *RecommendationEngine) *ViralOpportunityService {
	return &ViralOpportunityService{db: db, recommendationEngine: recommendationEngine}
}

// List returns paginated opportunities.
func (s *ViralOpportunityService) List(ctx context.Context, filters ViralOpportunityFilters) ([]models.ViralOpportunity, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.ViralOpportunity{})
	if filters.Category != "" {
		query = query.Where("LOWER(category) = ?", strings.ToLower(filters.Category))
	}
	if filters.Query != "" {
		like := "%" + strings.ToLower(filters.Query) + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(source_query) LIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var opportunities []models.ViralOpportunity
	if err := query.
		Order("viral_score DESC, view_velocity DESC, published_at DESC").
		Offset((filters.Page - 1) * filters.Limit).
		Limit(filters.Limit).
		Find(&opportunities).Error; err != nil {
		return nil, 0, err
	}

	return opportunities, total, nil
}

// Trending returns the hottest recent opportunities.
func (s *ViralOpportunityService) Trending(ctx context.Context, limit int) ([]models.ViralOpportunity, error) {
	cutoff := time.Now().UTC().Add(-trendingWindow)
	var opportunities []models.ViralOpportunity
	if err := s.db.WithContext(ctx).
		Where("published_at >= ?", cutoff).
		Order("viral_score DESC, growth_score DESC, view_velocity DESC").
		Limit(limit).
		Find(&opportunities).Error; err != nil {
		return nil, err
	}
	return opportunities, nil
}

// RecommendForUser returns the top personalized opportunity matches.
func (s *ViralOpportunityService) RecommendForUser(ctx context.Context, userID string, limit int) ([]dto.ViralOpportunityRecommendationResponse, error) {
	var profiles []models.ContentProfile
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&profiles).Error; err != nil {
		return nil, err
	}

	var opportunities []models.ViralOpportunity
	if err := s.db.WithContext(ctx).
		Where("published_at >= ?", time.Now().UTC().Add(-recommendationWindow)).
		Order("viral_score DESC, growth_score DESC").
		Limit(200).
		Find(&opportunities).Error; err != nil {
		return nil, err
	}

	return s.recommendationEngine.Recommend(opportunities, profiles, limit), nil
}

// RecommendationEngine ranks opportunities against user content profiles.
type RecommendationEngine struct{}

// NewRecommendationEngine creates a recommendation engine.
func NewRecommendationEngine() *RecommendationEngine {
	return &RecommendationEngine{}
}

// Recommend returns the strongest profile matches.
func (e *RecommendationEngine) Recommend(opportunities []models.ViralOpportunity, profiles []models.ContentProfile, limit int) []dto.ViralOpportunityRecommendationResponse {
	results := make([]dto.ViralOpportunityRecommendationResponse, 0, len(opportunities))
	for _, opportunity := range opportunities {
		reasons, matchedProfiles, fitScore := scoreOpportunity(opportunity, profiles)
		if len(profiles) > 0 && fitScore == 0 {
			continue
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "High viral score across recently collected YouTube videos")
		}
		recommendationScore := roundScore(opportunity.ViralScore*0.7 + fitScore)
		results = append(results, dto.ViralOpportunityRecommendationResponse{
			Opportunity:         toOpportunityResponse(opportunity),
			RecommendationScore: recommendationScore,
			Reasons:             reasons,
			MatchedProfiles:     matchedProfiles,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].RecommendationScore == results[j].RecommendationScore {
			return results[i].Opportunity.ViralScore > results[j].Opportunity.ViralScore
		}
		return results[i].RecommendationScore > results[j].RecommendationScore
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func scoreOpportunity(opportunity models.ViralOpportunity, profiles []models.ContentProfile) ([]string, []string, float64) {
	if len(profiles) == 0 {
		return []string{"High viral score across recently collected YouTube videos"}, nil, defaultFitScore
	}

	title := strings.ToLower(opportunity.Title)
	category := strings.ToLower(opportunity.Category)
	sourceQuery := strings.ToLower(opportunity.SourceQuery)
	reasons := make([]string, 0, 4)
	matchedProfiles := make([]string, 0, len(profiles))
	seenReasons := map[string]struct{}{}
	fitScore := 0.0

	for _, profile := range profiles {
		profileMatched := false
		if niche := strings.ToLower(strings.TrimSpace(profile.Niche)); niche != "" {
			if strings.Contains(category, niche) || strings.Contains(title, niche) || strings.Contains(sourceQuery, niche) {
				fitScore += 18
				profileMatched = true
				addReason(seenReasons, &reasons, "Matches profile niche: "+profile.Niche)
			}
		}

		keywords := splitKeywords(profile.Keywords)
		matchedKeywords := make([]string, 0, len(keywords))
		for _, keyword := range keywords {
			lowerKeyword := strings.ToLower(keyword)
			if strings.Contains(title, lowerKeyword) || strings.Contains(sourceQuery, lowerKeyword) || strings.Contains(category, lowerKeyword) {
				matchedKeywords = append(matchedKeywords, keyword)
			}
		}
		if len(matchedKeywords) > 0 {
			fitScore += float64(len(matchedKeywords) * 8)
			profileMatched = true
			addReason(seenReasons, &reasons, "Contains profile keywords: "+strings.Join(matchedKeywords, ", "))
		}

		platform := strings.ToLower(strings.TrimSpace(profile.Platform))
		if profileMatched && (platform == "youtube" || platform == "general" || platform == "") {
			fitScore += 5
		}

		if profileMatched {
			matchedProfiles = append(matchedProfiles, profile.Name)
		}
	}

	if len(matchedProfiles) == 0 {
		return nil, nil, 0
	}
	if opportunity.OutlierScore >= 2 {
		fitScore += 8
		addReason(seenReasons, &reasons, "Strong outlier score versus channel subscribers")
	}
	if opportunity.EngagementRate >= 0.05 {
		fitScore += 6
		addReason(seenReasons, &reasons, "Above-average engagement rate")
	}
	if opportunity.GrowthScore > 0 {
		fitScore += 5
		addReason(seenReasons, &reasons, "Momentum increased since the previous collection cycle")
	}

	return reasons, matchedProfiles, roundScore(fitScore)
}

func addReason(seen map[string]struct{}, reasons *[]string, reason string) {
	if _, ok := seen[reason]; ok {
		return
	}
	seen[reason] = struct{}{}
	*reasons = append(*reasons, reason)
}

func splitKeywords(raw string) []string {
	parts := strings.Split(raw, ",")
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		keywords = append(keywords, part)
	}
	return keywords
}

func roundScore(value float64) float64 {
	return math.Round(value*10000) / 10000
}

// ToOpportunityResponse converts the model into an API DTO.
func ToOpportunityResponse(opportunity models.ViralOpportunity) dto.ViralOpportunityResponse {
	return toOpportunityResponse(opportunity)
}

func toOpportunityResponse(opportunity models.ViralOpportunity) dto.ViralOpportunityResponse {
	return dto.ViralOpportunityResponse{
		ID:              opportunity.ID,
		SourcePlatform:  opportunity.SourcePlatform,
		ExternalVideoID: opportunity.ExternalVideoID,
		ChannelID:       opportunity.ChannelID,
		Title:           opportunity.Title,
		Category:        opportunity.Category,
		SourceQuery:     opportunity.SourceQuery,
		Views:           opportunity.Views,
		PreviousViews:   opportunity.PreviousViews,
		Likes:           opportunity.Likes,
		Comments:        opportunity.Comments,
		SubscriberCount: opportunity.SubscriberCount,
		PublishedAt:     opportunity.PublishedAt,
		LastCollectedAt: opportunity.LastCollectedAt,
		ViewVelocity:    opportunity.ViewVelocity,
		EngagementRate:  opportunity.EngagementRate,
		OutlierScore:    opportunity.OutlierScore,
		GrowthScore:     opportunity.GrowthScore,
		ViralScore:      opportunity.ViralScore,
		CreatedAt:       opportunity.CreatedAt,
		UpdatedAt:       opportunity.UpdatedAt,
	}
}
