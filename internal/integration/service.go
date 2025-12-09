package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gopost/integration/internal/config"
	"github.com/gopost/integration/internal/dedup"
	"github.com/gopost/integration/internal/drupal"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

type Service struct {
	esClient    *elasticsearch.Client
	drupal      *drupal.Client
	dedup       *dedup.Tracker
	limiter     *rate.Limiter
	config      *config.Config
	lastCheckTS time.Time
	mu          sync.RWMutex
}

func NewService(cfg *config.Config) (*Service, error) {
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
	drupalClient, err := drupal.NewClient(cfg.Drupal.URL, cfg.Drupal.Token)
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

	dedupTracker := dedup.NewTracker(redisClient)

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
		lastCheckTS: lastCheckTS,
	}, nil
}

type Article struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
	Source      string    `json:"source"`
}

func (s *Service) FindCrimeArticles(ctx context.Context, cityCfg config.CityConfig) ([]Article, error) {
	// Build Elasticsearch query
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"published_at": map[string]interface{}{
								"gte": s.getLastCheckTS().Format(time.RFC3339),
							},
						},
					},
					{
						"multi_match": map[string]interface{}{
							"query":    strings.Join(s.config.Service.CrimeKeywords, " "),
							"fields":   []string{"title^2", "content"},
							"type":     "best_fields",
							"operator": "or",
						},
					},
				},
			},
		},
		"size": 100,
		"sort": []map[string]interface{}{
			{
				"published_at": map[string]interface{}{
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

	res, err := s.esClient.Search(
		s.esClient.Search.WithContext(ctx),
		s.esClient.Search.WithIndex(index),
		s.esClient.Search.WithBody(&buf),
		s.esClient.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, fmt.Errorf("search error: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			return nil, fmt.Errorf("elasticsearch error response: %s", res.Status())
		}
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
	for _, hit := range result.Hits.Hits {
		// Use Elasticsearch _id if article doesn't have an ID
		if hit.Source.ID == "" {
			hit.Source.ID = hit.ID
		}
		articles = append(articles, hit.Source)
	}

	log.Printf("Found %d articles in %s (total: %d)", len(articles), cityCfg.Name, result.Hits.Total.Value)
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
	articles, err := s.FindCrimeArticles(ctx, cityCfg)
	if err != nil {
		return fmt.Errorf("find articles: %w", err)
	}

	posted := 0
	skipped := 0
	errors := 0

	for _, article := range articles {
		// Additional crime filtering
		if !s.isCrimeRelated(article) {
			skipped++
			continue
		}

		// Check if already posted
		if s.dedup.HasPosted(ctx, article.ID) {
			skipped++
			continue
		}

		// Rate limit
		if err := s.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}

		// Post to Drupal
		if err := s.drupal.PostArticle(ctx, drupal.ArticleRequest{
			Title:       article.Title,
			Body:        article.Content,
			URL:         article.URL,
			GroupID:     cityCfg.GroupID,
			GroupType:   s.config.Service.GroupType,
			ContentType: s.config.Service.ContentType,
		}); err != nil {
			log.Printf("Error posting article %s: %v", article.ID, err)
			errors++
			continue
		}

		// Mark as posted
		if err := s.dedup.MarkPosted(ctx, article.ID); err != nil {
			log.Printf("Warning: failed to mark article %s as posted: %v", article.ID, err)
		}

		posted++
		log.Printf("Posted article: %s (from %s)", article.Title, cityCfg.Name)
	}

	log.Printf("City %s: posted=%d, skipped=%d, errors=%d", cityCfg.Name, posted, skipped, errors)
	return nil
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.config.Service.CheckInterval)
	defer ticker.Stop()

	// Run immediately on start
	if err := s.runOnce(ctx); err != nil {
		log.Printf("Initial run error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.runOnce(ctx); err != nil {
				log.Printf("Run error: %v", err)
			}
		}
	}
}

func (s *Service) runOnce(ctx context.Context) error {
	log.Println("Starting article sync...")

	for _, cityCfg := range s.config.Cities {
		if err := s.ProcessCity(ctx, cityCfg); err != nil {
			log.Printf("Error processing city %s: %v", cityCfg.Name, err)
			// Continue with other cities
		}
	}

	// Update last check timestamp
	s.mu.Lock()
	s.lastCheckTS = time.Now()
	s.mu.Unlock()

	log.Println("Article sync completed")
	return nil
}

func (s *Service) getLastCheckTS() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheckTS
}
