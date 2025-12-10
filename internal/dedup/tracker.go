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

func NewTracker(client *redis.Client, ttl time.Duration, log logger.Logger) *Tracker {
	return &Tracker{
		client: client,
		ttl:    ttl,
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

// FlushAll removes all posted article keys from Redis
// This will clear the entire deduplication cache
func (t *Tracker) FlushAll(ctx context.Context) error {
	t.logger.Info("Flushing all posted article keys from Redis cache")

	// Use SCAN to find all keys matching the pattern "posted:article:*"
	// This is safer than FLUSHDB which would clear the entire Redis database
	pattern := "posted:article:*"
	var cursor uint64
	var deletedCount int

	for {
		var keys []string
		var err error
		const scanBatchSize = 100
		keys, cursor, err = t.client.Scan(ctx, cursor, pattern, scanBatchSize).Result()
		if err != nil {
			t.logger.Error("Redis error scanning for keys",
				logger.String("pattern", pattern),
				logger.Error(err),
			)
			return fmt.Errorf("scan keys: %w", err)
		}

		if len(keys) > 0 {
			deleted, delErr := t.client.Del(ctx, keys...).Result()
			if delErr != nil {
				t.logger.Error("Redis error deleting keys",
					logger.Int("key_count", len(keys)),
					logger.Error(delErr),
				)
				return fmt.Errorf("delete keys: %w", delErr)
			}
			deletedCount += int(deleted)
		}

		if cursor == 0 {
			break
		}
	}

	t.logger.Info("Flushed Redis cache",
		logger.Int("keys_deleted", deletedCount),
		logger.String("pattern", pattern),
	)

	return nil
}
