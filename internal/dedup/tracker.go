package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/gopost/integration/internal/logger"
	"github.com/redis/go-redis/v9"
)

type Tracker struct {
	client *redis.Client
	ttl    time.Duration
	logger logger.Logger
}

func NewTracker(client *redis.Client, log logger.Logger) *Tracker {
	return &Tracker{
		client: client,
		ttl:    365 * 24 * time.Hour, // Keep for 1 year
		logger: log,
	}
}

func (t *Tracker) key(articleID string) string {
	return fmt.Sprintf("posted:article:%s", articleID)
}

func (t *Tracker) HasPosted(ctx context.Context, articleID string) bool {
	key := t.key(articleID)

	t.logger.Debug("Checking if article was posted",
		logger.String("article_id", articleID),
		logger.String("redis_key", key),
	)

	exists, err := t.client.Exists(ctx, key).Result()
	if err != nil {
		t.logger.Error("Redis error checking article",
			logger.String("article_id", articleID),
			logger.String("redis_key", key),
			logger.Error(err),
		)
		// Log error but don't fail - assume not posted
		return false
	}

	alreadyPosted := exists == 1
	if alreadyPosted {
		t.logger.Debug("Article already posted",
			logger.String("article_id", articleID),
			logger.String("redis_key", key),
		)
	} else {
		t.logger.Debug("Article not yet posted",
			logger.String("article_id", articleID),
			logger.String("redis_key", key),
		)
	}

	return alreadyPosted
}

func (t *Tracker) MarkPosted(ctx context.Context, articleID string) error {
	key := t.key(articleID)

	t.logger.Debug("Marking article as posted",
		logger.String("article_id", articleID),
		logger.String("redis_key", key),
		logger.Duration("ttl", t.ttl),
	)

	err := t.client.Set(ctx, key, "1", t.ttl).Err()
	if err != nil {
		t.logger.Error("Redis error marking article as posted",
			logger.String("article_id", articleID),
			logger.String("redis_key", key),
			logger.Duration("ttl", t.ttl),
			logger.Error(err),
		)
		return err
	}

	t.logger.Debug("Article marked as posted",
		logger.String("article_id", articleID),
		logger.String("redis_key", key),
	)

	return nil
}

func (t *Tracker) Clear(ctx context.Context, articleID string) error {
	key := t.key(articleID)

	t.logger.Debug("Clearing article from posted cache",
		logger.String("article_id", articleID),
		logger.String("redis_key", key),
	)

	err := t.client.Del(ctx, key).Err()
	if err != nil {
		t.logger.Error("Redis error clearing article",
			logger.String("article_id", articleID),
			logger.String("redis_key", key),
			logger.Error(err),
		)
		return err
	}

	t.logger.Debug("Article cleared from posted cache",
		logger.String("article_id", articleID),
		logger.String("redis_key", key),
	)

	return nil
}
