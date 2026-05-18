package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// VideoStatusChannel is the Redis Pub/Sub channel pattern used to broadcast
// pipeline stage-change events to the API's WebSocket hub.
// Format: video:status:{videoID}
const VideoStatusChannel = "video:status:"

// StageChangeEvent is the payload published to Redis Pub/Sub after each
// pipeline stage transition. The API's StatusBroadcaster decodes this and
// forwards it to the connected WebSocket client.
type StageChangeEvent struct {
	VideoID     string    `json:"video_id"`
	UserID      string    `json:"user_id"`
	Stage       string    `json:"stage"`
	Status      string    `json:"status"`
	VideoStatus string    `json:"video_status"`
	Timestamp   time.Time `json:"timestamp"`
}

// StatusPublisher publishes stage-change events to Redis Pub/Sub.
type StatusPublisher struct {
	rdb *redis.Client
}

// NewStatusPublisher creates a StatusPublisher. rdb may be nil, in which case
// all publish operations are no-ops (useful in tests without Redis).
func NewStatusPublisher(rdb *redis.Client) *StatusPublisher {
	return &StatusPublisher{rdb: rdb}
}

// Publish sends a stage-change event to the Redis Pub/Sub channel for videoID.
func (p *StatusPublisher) Publish(ctx context.Context, event StageChangeEvent) {
	if p.rdb == nil {
		return
	}

	event.Timestamp = time.Now().UTC()
	data, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Str("video_id", event.VideoID).Msg("StatusPublisher: failed to marshal event")
		return
	}

	channel := VideoStatusChannel + event.VideoID
	if err := p.rdb.Publish(ctx, channel, string(data)).Err(); err != nil {
		log.Warn().Err(err).Str("channel", channel).Msg("StatusPublisher: failed to publish event")
		return
	}

	log.Debug().
		Str("video_id", event.VideoID).
		Str("stage", event.Stage).
		Str("status", event.Status).
		Str("channel", channel).
		Msg("StatusPublisher: event published")
}

// channelForVideo returns the Pub/Sub channel name for a videoID.
func channelForVideo(videoID string) string {
	return fmt.Sprintf("%s%s", VideoStatusChannel, videoID)
}
