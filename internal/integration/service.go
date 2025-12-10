package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gopost/integration/internal/config"
	"github.com/gopost/integration/internal/dedup"
	"github.com/gopost/integration/internal/drupal"
	"github.com/gopost/integration/internal/logger"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// Elasticsearch field name constants
const (
	ESFieldPublishedDate = "published_date"
	ESFieldBody          = "body"
	ESFieldCanonicalURL  = "canonical_url"
	ESFieldTitle         = "title"
	ESFieldSource        = "source"
)

// Timeout constants for external operations
const (
	esQueryTimeout    = 30 * time.Second
	drupalPostTimeout = 30 * time.Second
	redisTimeout      = 5 * time.Second
)

type Service struct {
	esClient    *elasticsearch.Client
	drupal      *drupal.Client
	dedup       *dedup.Tracker
	limiter     *rate.Limiter
	config      *config.Config
	logger      logger.Logger
	lastCheckTS time.Time
	mu          sync.RWMutex
}

func NewService(cfg *config.Config, log logger.Logger) (*Service, error) {
	// Initialize Elasticsearch client
	esCfg := elasticsearch.Config{
		Addresses: []string{cfg.Elasticsearch.URL},
	}
	if cfg.Elasticsearch.Username != "" {
		esCfg.Username = cfg.Elasticsearch.Username
		esCfg.Password = cfg.Elasticsearch.Password
	}

	esClient, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch client: %w", err)
	}

	// Initialize Drupal client
	drupalClient, err := drupal.NewClient(cfg.Drupal.URL, cfg.Drupal.Username, cfg.Drupal.Token, cfg.Drupal.AuthMethod, cfg.Drupal.SkipTLSVerify, log)
	if err != nil {
		return nil, fmt.Errorf("drupal client: %w", err)
	}

	// Initialize Redis for deduplication
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.URL,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection: %w", err)
	}

	dedupTracker := dedup.NewTracker(redisClient, cfg.Service.DedupTTL, log)

	// Initialize rate limiter
	limiter := rate.NewLimiter(rate.Limit(cfg.Service.RateLimitRPS), cfg.Service.RateLimitRPS)

	// Set initial last check time
	lookbackDuration := time.Duration(cfg.Service.LookbackHours) * time.Hour
	lastCheckTS := time.Now().Add(-lookbackDuration)

	return &Service{
		esClient:    esClient,
		drupal:      drupalClient,
		dedup:       dedupTracker,
		limiter:     limiter,
		config:      cfg,
		logger:      log,
		lastCheckTS: lastCheckTS,
	}, nil
}

type Article struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`          // Maps to ESFieldTitle
	Content       string    `json:"body"`           // Maps to ESFieldBody
	URL           string    `json:"canonical_url"`  // Maps to ESFieldCanonicalURL
	PublishedAt   time.Time `json:"published_date"` // Maps to ESFieldPublishedDate
	Source        string    `json:"source"`         // Maps to ESFieldSource
	Intro         string    `json:"intro,omitempty"`
	Description   string    `json:"description,omitempty"`
	OGTitle       string    `json:"og_title,omitempty"`
	OGDescription string    `json:"og_description,omitempty"`
	OGImage       string    `json:"og_image,omitempty"`
	OGURL         string    `json:"og_url,omitempty"`
	WordCount     int       `json:"word_count,omitempty"`
	Category      string    `json:"category,omitempty"`
	Section       string    `json:"section,omitempty"`
	Keywords      []string  `json:"keywords,omitempty"`
}

func (s *Service) FindCrimeArticles(ctx context.Context, cityCfg config.CityConfig) ([]Article, error) {
	startTime := time.Now()

	// Build Elasticsearch query
	mustClauses := []map[string]any{
		{
			"multi_match": map[string]any{
				"query":    strings.Join(s.config.Service.CrimeKeywords, " "),
				"fields":   []string{ESFieldTitle + "^2", ESFieldBody},
				"type":     "best_fields",
				"operator": "or",
			},
		},
	}

	// Add date filter only if lookback_hours is positive
	if s.config.Service.LookbackHours > 0 {
		lastCheckTS := s.getLastCheckTS()
		lastCheckStr := lastCheckTS.Format(time.RFC3339)
		s.logger.Debug("Searching for articles with date filter",
			logger.String("city", cityCfg.Name),
			logger.String("since", lastCheckStr),
			logger.Int("lookback_hours", s.config.Service.LookbackHours),
		)

		mustClauses = append([]map[string]any{
			{
				"range": map[string]any{
					ESFieldPublishedDate: map[string]any{
						"gte": lastCheckStr,
					},
				},
			},
		}, mustClauses...)
	} else {
		s.logger.Debug("Searching for articles without date filter",
			logger.String("city", cityCfg.Name),
			logger.Int("lookback_hours", s.config.Service.LookbackHours),
		)
	}

	query := map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"must": mustClauses,
			},
		},
		"size": 100,
		"sort": []map[string]any{
			{
				ESFieldPublishedDate: map[string]any{
					"order": "desc",
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, fmt.Errorf("encode query: %w", err)
	}

	// Execute search
	index := cityCfg.Index
	if index == "" {
		index = fmt.Sprintf("%s_articles", cityCfg.Name)
	}

	// Log the query for debugging
	queryJSON, _ := json.MarshalIndent(query, "", "  ")
	s.logger.Debug("Elasticsearch query",
		logger.String("query", string(queryJSON)),
		logger.String("index_name", index),
		logger.String("city", cityCfg.Name),
	)

	// Create context with timeout for Elasticsearch query
	queryCtx, queryCancel := context.WithTimeout(ctx, esQueryTimeout)
	defer queryCancel()

	queryStartTime := time.Now()
	res, err := s.esClient.Search(
		s.esClient.Search.WithContext(queryCtx),
		s.esClient.Search.WithIndex(index),
		s.esClient.Search.WithBody(&buf),
		s.esClient.Search.WithTrackTotalHits(true),
	)
	queryDuration := time.Since(queryStartTime)

	if err != nil {
		s.logger.Error("Elasticsearch search failed",
			logger.String("index_name", index),
			logger.String("city", cityCfg.Name),
			logger.Duration("query_duration", queryDuration),
			logger.Error(err),
		)
		return nil, fmt.Errorf("search error: %w", err)
	}
	defer res.Body.Close()

	s.logger.Debug("Elasticsearch query completed",
		logger.String("index_name", index),
		logger.String("city", cityCfg.Name),
		logger.Duration("query_duration", queryDuration),
		logger.String("status", res.Status()),
	)

	if res.IsError() {
		var e map[string]any
		if decodeErr := json.NewDecoder(res.Body).Decode(&e); decodeErr != nil {
			s.logger.Error("Failed to decode Elasticsearch error response",
				logger.String("index_name", index),
				logger.String("city", cityCfg.Name),
				logger.String("status", res.Status()),
				logger.Error(decodeErr),
			)
			return nil, fmt.Errorf("elasticsearch error response: %s", res.Status())
		}
		s.logger.Error("Elasticsearch error",
			logger.String("index_name", index),
			logger.String("city", cityCfg.Name),
			logger.String("status", res.Status()),
			logger.Duration("query_duration", queryDuration),
			logger.Any("error_details", e),
		)
		return nil, fmt.Errorf("elasticsearch error: %v", e)
	}

	var result struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string  `json:"_id"`
				Source Article `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	articles := make([]Article, 0, len(result.Hits.Hits))
	for i := range result.Hits.Hits {
		hit := &result.Hits.Hits[i]
		// Use Elasticsearch _id if article doesn't have an ID
		if hit.Source.ID == "" {
			hit.Source.ID = hit.ID
		}
		articles = append(articles, hit.Source)
	}

	totalDuration := time.Since(startTime)
	s.logger.Info("Found articles",
		logger.String("city", cityCfg.Name),
		logger.String("index_name", index),
		logger.Int("count", len(articles)),
		logger.Int("total", result.Hits.Total.Value),
		logger.Duration("duration", totalDuration),
		logger.Duration("query_duration", queryDuration),
	)

	// If no articles found, log a sample query without keyword filter for debugging
	if result.Hits.Total.Value == 0 && len(s.config.Service.CrimeKeywords) > 0 {
		s.logger.Debug("No articles found, testing query without keyword filter",
			logger.String("city", cityCfg.Name),
			logger.String("index_name", index),
		)
		testQuery := map[string]any{
			"query": map[string]any{
				"match_all": map[string]any{},
			},
			"size": 1,
		}
		var testBuf bytes.Buffer
		if err := json.NewEncoder(&testBuf).Encode(testQuery); err == nil {
			testRes, err := s.esClient.Search(
				s.esClient.Search.WithContext(ctx),
				s.esClient.Search.WithIndex(index),
				s.esClient.Search.WithBody(&testBuf),
				s.esClient.Search.WithTrackTotalHits(true),
			)
			if err == nil {
				defer testRes.Body.Close()
				if !testRes.IsError() {
					var testResult struct {
						Hits struct {
							Total struct {
								Value int `json:"value"`
							} `json:"total"`
							Hits []struct {
								Source map[string]any `json:"_source"`
							} `json:"hits"`
						} `json:"hits"`
					}
					if err := json.NewDecoder(testRes.Body).Decode(&testResult); err == nil {
						s.logger.Debug("Index contains articles without filters",
							logger.String("index_name", index),
							logger.String("city", cityCfg.Name),
							logger.Int("total_articles", testResult.Hits.Total.Value),
						)
						if len(testResult.Hits.Hits) > 0 {
							s.logger.Debug("Sample article fields",
								logger.String("index_name", index),
								logger.String("city", cityCfg.Name),
								logger.Any("sample_fields", testResult.Hits.Hits[0].Source),
							)
						}
					} else {
						s.logger.Debug("Failed to decode test query result",
							logger.String("index_name", index),
							logger.String("city", cityCfg.Name),
							logger.Error(err),
						)
					}
				}
			}
		}
	}

	return articles, nil
}

func (s *Service) isCrimeRelated(article Article) bool {
	content := strings.ToLower(article.Title + " " + article.Content)
	for _, keyword := range s.config.Service.CrimeKeywords {
		if strings.Contains(content, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func (s *Service) ProcessCity(ctx context.Context, cityCfg config.CityConfig) error {
	startTime := time.Now()

	articles, err := s.FindCrimeArticles(ctx, cityCfg)
	if err != nil {
		s.logger.Error("Failed to find articles",
			logger.String("city", cityCfg.Name),
			logger.Error(err),
		)
		return fmt.Errorf("find articles: %w", err)
	}

	posted := 0
	skipped := 0
	errors := 0

	s.logger.Debug("Processing articles",
		logger.String("city", cityCfg.Name),
		logger.Int("article_count", len(articles)),
	)

	for i := range articles {
		article := &articles[i]
		articleStartTime := time.Now()

		// Additional crime filtering
		if !s.isCrimeRelated(*article) {
			s.logger.Debug("Article skipped - not crime related",
				logger.String("article_id", article.ID),
				logger.String("city", cityCfg.Name),
				logger.String("title", article.Title),
				logger.Int("article_index", i+1),
			)
			skipped++
			continue
		}

		// Check if already posted (with timeout)
		dedupCtx, dedupCancel := context.WithTimeout(ctx, redisTimeout)
		dedupStartTime := time.Now()
		alreadyPosted := s.dedup.HasPosted(dedupCtx, article.ID)
		dedupDuration := time.Since(dedupStartTime)
		dedupCancel()

		s.logger.Debug("Deduplication check",
			logger.String("article_id", article.ID),
			logger.String("city", cityCfg.Name),
			logger.Bool("already_posted", alreadyPosted),
			logger.Duration("dedup_duration", dedupDuration),
		)

		if alreadyPosted {
			s.logger.Debug("Article skipped - already posted",
				logger.String("article_id", article.ID),
				logger.String("city", cityCfg.Name),
				logger.String("title", article.Title),
			)
			skipped++
			continue
		}

		// Rate limit
		rateLimitStartTime := time.Now()
		if err := s.limiter.Wait(ctx); err != nil {
			s.logger.Error("Rate limit wait failed",
				logger.String("article_id", article.ID),
				logger.String("city", cityCfg.Name),
				logger.Error(err),
			)
			return fmt.Errorf("rate limit wait: %w", err)
		}
		rateLimitDuration := time.Since(rateLimitStartTime)

		s.logger.Debug("Rate limit wait completed",
			logger.String("article_id", article.ID),
			logger.String("city", cityCfg.Name),
			logger.Duration("rate_limit_wait_duration", rateLimitDuration),
		)

		// Post to Drupal (with timeout)
		postCtx, postCancel := context.WithTimeout(ctx, drupalPostTimeout)
		postStartTime := time.Now()
		// Derive OG fields from canonical fields if not present (DRY principle)
		// After crawler refactor: OG fields are only stored in ES if they differ from canonical values.
		// If present in ES, use them; otherwise derive from canonical fields.
		ogTitle := article.OGTitle
		if ogTitle == "" {
			ogTitle = article.Title
		}
		ogDescription := article.OGDescription
		if ogDescription == "" {
			// Prefer description, fallback to intro
			if article.Description != "" {
				ogDescription = article.Description
			} else {
				ogDescription = article.Intro
			}
		}
		ogURL := article.OGURL
		if ogURL == "" {
			// Prefer canonical_url, fallback to source
			if article.URL != "" {
				ogURL = article.URL
			} else {
				ogURL = article.Source
			}
		}

		postErr := s.drupal.PostArticle(postCtx, drupal.ArticleRequest{
			Title:         article.Title,
			Body:          article.Content,
			URL:           article.URL,
			GroupID:       cityCfg.GroupID,
			GroupType:     s.config.Service.GroupType,
			ContentType:   s.config.Service.ContentType,
			ExternalID:    article.ID,
			Intro:         article.Intro,
			Description:   article.Description,
			OGTitle:       ogTitle,
			OGDescription: ogDescription,
			OGImage:       article.OGImage, // og_image is unique, not duplicated
			OGURL:         ogURL,
			WordCount:     article.WordCount,
			Category:      article.Category,
			Section:       article.Section,
			Keywords:      article.Keywords,
			CanonicalURL:  article.URL, // canonical_url is the same as URL in our case
			PublishedDate: article.PublishedAt,
		})
		postCancel()
		if postErr != nil {
			postDuration := time.Since(postStartTime)
			articleDuration := time.Since(articleStartTime)
			s.logger.Error("Error posting article",
				logger.String("article_id", article.ID),
				logger.String("city", cityCfg.Name),
				logger.String("title", article.Title),
				logger.String("url", article.URL),
				logger.Duration("post_duration", postDuration),
				logger.Duration("article_processing_duration", articleDuration),
				logger.Error(postErr),
			)
			errors++
			continue
		}
		postDuration := time.Since(postStartTime)

		// Mark as posted (with timeout)
		markCtx, markCancel := context.WithTimeout(ctx, redisTimeout)
		markStartTime := time.Now()
		markErr := s.dedup.MarkPosted(markCtx, article.ID)
		markCancel()
		if markErr != nil {
			markDuration := time.Since(markStartTime)
			s.logger.Warn("Failed to mark article as posted",
				logger.String("article_id", article.ID),
				logger.String("city", cityCfg.Name),
				logger.Duration("mark_duration", markDuration),
				logger.Error(markErr),
			)
		} else {
			markDuration := time.Since(markStartTime)
			s.logger.Debug("Article marked as posted",
				logger.String("article_id", article.ID),
				logger.String("city", cityCfg.Name),
				logger.Duration("mark_duration", markDuration),
			)
		}

		posted++
		articleDuration := time.Since(articleStartTime)
		s.logger.Info("Posted article",
			logger.String("title", article.Title),
			logger.String("city", cityCfg.Name),
			logger.String("article_id", article.ID),
			logger.String("url", article.URL),
			logger.Duration("post_duration", postDuration),
			logger.Duration("article_processing_duration", articleDuration),
			logger.Int("article_index", i+1),
			logger.Int("total_articles", len(articles)),
		)
	}

	totalDuration := time.Since(startTime)
	s.logger.Info("City processing completed",
		logger.String("city", cityCfg.Name),
		logger.Int("posted", posted),
		logger.Int("skipped", skipped),
		logger.Int("errors", errors),
		logger.Int("total_articles", len(articles)),
		logger.Duration("total_duration", totalDuration),
	)
	return nil
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.config.Service.CheckInterval)
	defer ticker.Stop()

	// Run immediately on start
	if err := s.runOnce(ctx); err != nil {
		s.logger.Error("Initial run error",
			logger.Error(err),
		)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.runOnce(ctx); err != nil {
				s.logger.Error("Run error",
					logger.Error(err),
				)
			}
		}
	}
}

func (s *Service) runOnce(ctx context.Context) error {
	startTime := time.Now()
	s.logger.Info("Starting article sync",
		logger.Int("city_count", len(s.config.Cities)),
	)

	for i, cityCfg := range s.config.Cities {
		cityStartTime := time.Now()
		s.logger.Debug("Processing city",
			logger.String("city", cityCfg.Name),
			logger.Int("city_index", i+1),
			logger.Int("total_cities", len(s.config.Cities)),
		)

		if err := s.ProcessCity(ctx, cityCfg); err != nil {
			cityDuration := time.Since(cityStartTime)
			s.logger.Error("Error processing city",
				logger.String("city", cityCfg.Name),
				logger.Int("city_index", i+1),
				logger.Duration("city_duration", cityDuration),
				logger.Error(err),
			)
			// Continue with other cities
		} else {
			cityDuration := time.Since(cityStartTime)
			s.logger.Debug("City processing completed",
				logger.String("city", cityCfg.Name),
				logger.Int("city_index", i+1),
				logger.Duration("city_duration", cityDuration),
			)
		}
	}

	// Update last check timestamp
	s.mu.Lock()
	s.lastCheckTS = time.Now()
	s.mu.Unlock()

	totalDuration := time.Since(startTime)
	s.logger.Info("Article sync completed",
		logger.Int("city_count", len(s.config.Cities)),
		logger.Duration("total_duration", totalDuration),
	)
	return nil
}

func (s *Service) getLastCheckTS() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheckTS
}

// FlushCache flushes the Redis deduplication cache
func (s *Service) FlushCache(ctx context.Context) error {
	return s.dedup.FlushAll(ctx)
}
