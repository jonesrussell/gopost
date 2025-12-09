package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Tracker struct {
	client *redis.Client
	ttl    time.Duration
}

func NewTracker(client *redis.Client) *Tracker {
	return &Tracker{
		client: client,
		ttl:    365 * 24 * time.Hour, // Keep for 1 year
	}
}

func (t *Tracker) key(articleID string) string {
	return fmt.Sprintf("posted:article:%s", articleID)
}

func (t *Tracker) HasPosted(ctx context.Context, articleID string) bool {
	key := t.key(articleID)
	exists, err := t.client.Exists(ctx, key).Result()
	if err != nil {
		// Log error but don't fail - assume not posted
		return false
	}
	return exists == 1
}

func (t *Tracker) MarkPosted(ctx context.Context, articleID string) error {
	key := t.key(articleID)
	return t.client.Set(ctx, key, "1", t.ttl).Err()
}

func (t *Tracker) Clear(ctx context.Context, articleID string) error {
	key := t.key(articleID)
	return t.client.Del(ctx, key).Err()
}
