package trends

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const sourcePlatformYouTube = "youtube"

// YouTubeCollector fetches recent high-performing videos from the YouTube Data API.
type YouTubeCollector struct {
	apiKey     string
	baseURL    string
	regionCode string
	maxResults int
	lookback   time.Duration
	httpClient *http.Client
}

// CollectedVideo represents the raw metrics pulled from YouTube.
type CollectedVideo struct {
	SourcePlatform  string
	ExternalVideoID string
	ChannelID       string
	Title           string
	Category        string
	SourceQuery     string
	Views           int64
	Likes           int64
	Comments        int64
	SubscriberCount int64
	PublishedAt     time.Time
}

// NewYouTubeCollector constructs a collector instance.
func NewYouTubeCollector(apiKey, regionCode string, maxResults int, lookback time.Duration, httpClient *http.Client) *YouTubeCollector {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if regionCode == "" {
		regionCode = "US"
	}
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 10
	}
	if lookback <= 0 {
		lookback = 7 * 24 * time.Hour
	}
	return &YouTubeCollector{
		apiKey:     apiKey,
		baseURL:    "https://www.googleapis.com/youtube/v3",
		regionCode: regionCode,
		maxResults: maxResults,
		lookback:   lookback,
		httpClient: httpClient,
	}
}

// WithBaseURL overrides the API base URL for tests.
func (c *YouTubeCollector) WithBaseURL(baseURL string) *YouTubeCollector {
	if baseURL != "" {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
	return c
}

// Enabled reports whether the collector is configured.
func (c *YouTubeCollector) Enabled() bool {
	return strings.TrimSpace(c.apiKey) != ""
}

// Collect fetches recent trend candidates for the supplied search queries.
func (c *YouTubeCollector) Collect(ctx context.Context, queries []string) ([]CollectedVideo, error) {
	if !c.Enabled() {
		return nil, ErrCollectorDisabled
	}
	if len(queries) == 0 {
		return nil, nil
	}

	categoryMap, err := c.fetchCategoryMap(ctx)
	if err != nil {
		return nil, err
	}

	videoQueryMap := make(map[string]string)
	for _, query := range queries {
		items, err := c.searchVideos(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, item := range items.Items {
			videoQueryMap[item.ID.VideoID] = query
		}
	}
	if len(videoQueryMap) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(videoQueryMap))
	for id := range videoQueryMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	videoDetails, err := c.fetchVideoDetails(ctx, ids)
	if err != nil {
		return nil, err
	}
	channelIDs := make([]string, 0, len(videoDetails))
	for _, detail := range videoDetails {
		if detail.Snippet.ChannelID != "" {
			channelIDs = append(channelIDs, detail.Snippet.ChannelID)
		}
	}
	subscriberMap, err := c.fetchChannelSubscribers(ctx, channelIDs)
	if err != nil {
		return nil, err
	}

	videos := make([]CollectedVideo, 0, len(videoDetails))
	for _, detail := range videoDetails {
		publishedAt, err := time.Parse(time.RFC3339, detail.Snippet.PublishedAt)
		if err != nil {
			continue
		}
		videos = append(videos, CollectedVideo{
			SourcePlatform:  sourcePlatformYouTube,
			ExternalVideoID: detail.ID,
			ChannelID:       detail.Snippet.ChannelID,
			Title:           detail.Snippet.Title,
			Category:        categoryMap[detail.Snippet.CategoryID],
			SourceQuery:     videoQueryMap[detail.ID],
			Views:           parseInt64(detail.Statistics.ViewCount),
			Likes:           parseInt64(detail.Statistics.LikeCount),
			Comments:        parseInt64(detail.Statistics.CommentCount),
			SubscriberCount: subscriberMap[detail.Snippet.ChannelID],
			PublishedAt:     publishedAt.UTC(),
		})
	}

	return videos, nil
}

type searchResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
	} `json:"items"`
}

type videosResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title       string `json:"title"`
			CategoryID  string `json:"categoryId"`
			ChannelID   string `json:"channelId"`
			PublishedAt string `json:"publishedAt"`
		} `json:"snippet"`
		Statistics struct {
			ViewCount    string `json:"viewCount"`
			LikeCount    string `json:"likeCount"`
			CommentCount string `json:"commentCount"`
		} `json:"statistics"`
	} `json:"items"`
}

type channelsResponse struct {
	Items []struct {
		ID         string `json:"id"`
		Statistics struct {
			SubscriberCount string `json:"subscriberCount"`
		} `json:"statistics"`
	} `json:"items"`
}

type categoriesResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
	} `json:"items"`
}

func (c *YouTubeCollector) searchVideos(ctx context.Context, query string) (*searchResponse, error) {
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("part", "snippet")
	params.Set("type", "video")
	params.Set("order", "viewCount")
	params.Set("maxResults", fmt.Sprintf("%d", c.maxResults))
	params.Set("regionCode", c.regionCode)
	params.Set("publishedAfter", time.Now().UTC().Add(-c.lookback).Format(time.RFC3339))
	params.Set("q", query)

	var resp searchResponse
	if err := c.get(ctx, "/search", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *YouTubeCollector) fetchVideoDetails(ctx context.Context, ids []string) ([]struct {
	ID      string `json:"id"`
	Snippet struct {
		Title       string `json:"title"`
		CategoryID  string `json:"categoryId"`
		ChannelID   string `json:"channelId"`
		PublishedAt string `json:"publishedAt"`
	} `json:"snippet"`
	Statistics struct {
		ViewCount    string `json:"viewCount"`
		LikeCount    string `json:"likeCount"`
		CommentCount string `json:"commentCount"`
	} `json:"statistics"`
}, error) {
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("part", "snippet,statistics")
	params.Set("id", strings.Join(ids, ","))
	params.Set("maxResults", fmt.Sprintf("%d", len(ids)))

	var resp videosResponse
	if err := c.get(ctx, "/videos", params, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *YouTubeCollector) fetchChannelSubscribers(ctx context.Context, channelIDs []string) (map[string]int64, error) {
	if len(channelIDs) == 0 {
		return map[string]int64{}, nil
	}
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("part", "statistics")
	params.Set("id", strings.Join(unique(channelIDs), ","))
	var resp channelsResponse
	if err := c.get(ctx, "/channels", params, &resp); err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(resp.Items))
	for _, item := range resp.Items {
		result[item.ID] = parseInt64(item.Statistics.SubscriberCount)
	}
	return result, nil
}

func (c *YouTubeCollector) fetchCategoryMap(ctx context.Context) (map[string]string, error) {
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("part", "snippet")
	params.Set("regionCode", c.regionCode)
	var resp categoriesResponse
	if err := c.get(ctx, "/videoCategories", params, &resp); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(resp.Items))
	for _, item := range resp.Items {
		result[item.ID] = item.Snippet.Title
	}
	return result, nil
}

func (c *YouTubeCollector) get(ctx context.Context, path string, params url.Values, target interface{}) error {
	endpoint := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request %s: %w", path, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request %s returned status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
