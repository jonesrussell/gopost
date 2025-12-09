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
	drupalClient, err := drupal.NewClient(cfg.Drupal.URL, cfg.Drupal.Token, cfg.Drupal.SkipTLSVerify)
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
	Content     string    `json:"body"`           // Elasticsearch uses "body" not "content"
	URL         string    `json:"canonical_url"`  // Elasticsearch uses "canonical_url" not "url"
	PublishedAt time.Time `json:"published_date"` // Elasticsearch uses "published_date" not "published_at"
	Source      string    `json:"source"`
}

func (s *Service) FindCrimeArticles(ctx context.Context, cityCfg config.CityConfig) ([]Article, error) {
	// Build Elasticsearch query
	mustClauses := []map[string]interface{}{
		{
			"multi_match": map[string]interface{}{
				"query":    strings.Join(s.config.Service.CrimeKeywords, " "),
				"fields":   []string{"title^2", "body"}, // Use "body" instead of "content"
				"type":     "best_fields",
				"operator": "or",
			},
		},
	}

	// Add date filter only if lookback_hours is positive
	if s.config.Service.LookbackHours > 0 {
		lastCheckTS := s.getLastCheckTS()
		lastCheckStr := lastCheckTS.Format(time.RFC3339)
		log.Printf("Searching for articles in %s since %s (lookback: %d hours)", cityCfg.Name, lastCheckStr, s.config.Service.LookbackHours)

		mustClauses = append([]map[string]interface{}{
			{
				"range": map[string]interface{}{
					"published_date": map[string]interface{}{ // Use "published_date" instead of "published_at"
						"gte": lastCheckStr,
					},
				},
			},
		}, mustClauses...)
	} else {
		log.Printf("Searching for articles in %s (no date filter, lookback: %d hours)", cityCfg.Name, s.config.Service.LookbackHours)
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustClauses,
			},
		},
		"size": 100,
		"sort": []map[string]interface{}{
			{
				"published_date": map[string]interface{}{ // Use "published_date" instead of "published_at"
					"order": "desc",
				},
			},
		},
	}

	// Log the query for debugging
	queryJSON, _ := json.MarshalIndent(query, "", "  ")
	log.Printf("Elasticsearch query: %s", string(queryJSON))

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
		log.Printf("Elasticsearch error details: %+v", e)
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

	// If no articles found, log a sample query without keyword filter for debugging
	if result.Hits.Total.Value == 0 && len(s.config.Service.CrimeKeywords) > 0 {
		log.Printf("No articles found. Testing query without keyword filter to check if articles exist...")
		testQuery := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
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
								Source map[string]interface{} `json:"_source"`
							} `json:"hits"`
						} `json:"hits"`
					}
					if err := json.NewDecoder(testRes.Body).Decode(&testResult); err == nil {
						log.Printf("Index %s contains %d total articles (without filters)", index, testResult.Hits.Total.Value)
						if len(testResult.Hits.Hits) > 0 {
							log.Printf("Sample article fields: %+v", testResult.Hits.Hits[0].Source)
						}
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
