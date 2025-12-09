package drupal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gopost/integration/internal/logger"
)

type Client struct {
	baseURL    string
	username   string
	token      string
	authMethod string
	client     *http.Client
	logger     logger.Logger
}

type ArticleRequest struct {
	Title       string
	Body        string
	URL         string
	GroupID     string
	GroupType   string
	ContentType string
}

type DrupalArticle struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Title string `json:"title"`
			Body  string `json:"body,omitempty"`
		} `json:"attributes"`
		Relationships struct {
			FieldGroup struct {
				Data struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"data"`
			} `json:"field_group"`
		} `json:"relationships"`
	} `json:"data"`
}

type DrupalResponse struct {
	Data struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"data"`
	Errors []DrupalError `json:"errors,omitempty"`
}

type DrupalError struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func NewClient(baseURL, username, token, authMethod string, skipTLSVerify bool, log logger.Logger) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("drupal URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("drupal token is required")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Skip TLS verification in development mode
	if skipTLSVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		log.Warn("TLS certificate verification is disabled",
			logger.String("base_url", baseURL),
			logger.String("component", "drupal_client"),
		)
	}

	return &Client{
		baseURL:    baseURL,
		username:   username,
		token:      token,
		authMethod: authMethod,
		client:     client,
		logger:     log,
	}, nil
}

// getCSRFToken fetches a CSRF token from Drupal's session/token endpoint
// Note: The session/token endpoint may require Basic Auth, while JSON:API uses API-KEY header
func (c *Client) getCSRFToken(ctx context.Context) (string, error) {
	tokenURL := fmt.Sprintf("%s/session/token", c.baseURL)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("create CSRF token request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")

	// Use API-KEY authentication for session/token endpoint
	var apiKeyValue string
	if c.username != "" {
		apiKeyValue = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.username, c.token)))
		httpReq.Header.Set("API-KEY", apiKeyValue)
		// Set Authorization header with Basic format (required by miniOrange)
		httpReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", apiKeyValue))
	} else {
		// Fallback: if no username, just use token (base64 encoded)
		apiKeyValue = base64.StdEncoding.EncodeToString([]byte(c.token))
		httpReq.Header.Set("API-KEY", apiKeyValue)
		httpReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", apiKeyValue))
	}

	// Include AUTH-METHOD header if configured (required by miniOrange REST API Authentication)
	if c.authMethod != "" {
		httpReq.Header.Set("AUTH-METHOD", c.authMethod)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("fetch CSRF token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("CSRF token request failed: %d %s", resp.StatusCode, resp.Status)
	}

	// CSRF token is returned as plain text
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read CSRF token: %w", err)
	}

	// Trim any whitespace and newlines
	csrfToken := strings.TrimSpace(string(bodyBytes))
	return csrfToken, nil
}

func (c *Client) PostArticle(ctx context.Context, req ArticleRequest) error {
	startTime := time.Now()

	// Add method-level context
	methodLogger := c.logger.With(
		logger.String("method", "PostArticle"),
	)

	drupalArticle := DrupalArticle{}
	drupalArticle.Data.Type = req.ContentType
	drupalArticle.Data.Attributes.Title = req.Title
	if req.Body != "" {
		drupalArticle.Data.Attributes.Body = req.Body
	}
	drupalArticle.Data.Relationships.FieldGroup.Data.Type = req.GroupType
	drupalArticle.Data.Relationships.FieldGroup.Data.ID = req.GroupID
	// Note: field_url is a relationship field in Drupal and requires a UUID reference
	// For now, we're omitting it. If needed, we'd need to create/find the URL entity first.

	payload, err := json.Marshal(drupalArticle)
	if err != nil {
		methodLogger.Error("Failed to marshal article payload",
			logger.String("title", req.Title),
			logger.String("content_type", req.ContentType),
			logger.Error(err),
		)
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Construct endpoint URL
	endpoint := fmt.Sprintf("%s/jsonapi/node/article", c.baseURL)
	if req.ContentType != "node--article" {
		// Extract content type from "node--article" format
		contentType := req.ContentType
		if len(contentType) > 5 && contentType[:5] == "node--" {
			contentType = contentType[5:]
		}
		endpoint = fmt.Sprintf("%s/jsonapi/node/%s", c.baseURL, contentType)
	}

	methodLogger.Debug("Posting article to Drupal",
		logger.String("endpoint", endpoint),
		logger.String("title", req.Title),
		logger.String("content_type", req.ContentType),
		logger.String("group_type", req.GroupType),
		logger.String("group_id", req.GroupID),
		logger.String("url", req.URL),
		logger.Int("payload_size", len(payload)),
	)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(payload))
	if err != nil {
		methodLogger.Error("Failed to create HTTP request",
			logger.String("endpoint", endpoint),
			logger.String("title", req.Title),
			logger.Error(err),
		)
		return fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/vnd.api+json")
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	// REST API Authentication module expects API-KEY header with base64(username:api-key)
	// Also include Authorization header with Basic format as miniOrange requires it
	var apiKeyValue string
	if c.username != "" {
		apiKeyValue = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.username, c.token)))
		httpReq.Header.Set("API-KEY", apiKeyValue)
		// Set Authorization header with Basic format (required by miniOrange)
		httpReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", apiKeyValue))
	} else {
		// Fallback: if no username, just use token (base64 encoded)
		apiKeyValue = base64.StdEncoding.EncodeToString([]byte(c.token))
		httpReq.Header.Set("API-KEY", apiKeyValue)
		httpReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", apiKeyValue))
	}

	// Include AUTH-METHOD header if configured (required by miniOrange REST API Authentication)
	if c.authMethod != "" {
		httpReq.Header.Set("AUTH-METHOD", c.authMethod)
	}

	// Fetch and include CSRF token for POST requests
	csrfToken, err := c.getCSRFToken(ctx)
	if err != nil {
		methodLogger.Warn("Failed to fetch CSRF token, proceeding without it",
			logger.String("endpoint", endpoint),
			logger.Error(err),
		)
	} else {
		httpReq.Header.Set("X-CSRF-Token", csrfToken)
		methodLogger.Debug("Included CSRF token in request",
			logger.String("endpoint", endpoint),
		)
	}

	requestStartTime := time.Now()
	resp, err := c.client.Do(httpReq)
	requestDuration := time.Since(requestStartTime)

	if err != nil {
		methodLogger.Error("HTTP request failed",
			logger.String("endpoint", endpoint),
			logger.String("title", req.Title),
			logger.Duration("request_duration", requestDuration),
			logger.Error(err),
		)
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var drupalResp DrupalResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&drupalResp)

		if decodeErr == nil && len(drupalResp.Errors) > 0 {
			errorDetail := drupalResp.Errors[0]
			methodLogger.Error("Drupal API error",
				logger.String("endpoint", endpoint),
				logger.String("article_title", req.Title),
				logger.Int("status_code", resp.StatusCode),
				logger.String("status", resp.Status),
				logger.String("error_status", errorDetail.Status),
				logger.String("error_title", errorDetail.Title),
				logger.String("error_detail", errorDetail.Detail),
				logger.Duration("request_duration", requestDuration),
			)
			return fmt.Errorf("drupal API error (%d): %s - %s",
				resp.StatusCode,
				errorDetail.Title,
				errorDetail.Detail)
		}

		methodLogger.Error("Drupal API error",
			logger.String("endpoint", endpoint),
			logger.String("article_title", req.Title),
			logger.Int("status_code", resp.StatusCode),
			logger.String("status", resp.Status),
			logger.Duration("request_duration", requestDuration),
			logger.Error(decodeErr),
		)
		return fmt.Errorf("drupal API error: %d %s", resp.StatusCode, resp.Status)
	}

	var drupalResp DrupalResponse
	if err := json.NewDecoder(resp.Body).Decode(&drupalResp); err != nil {
		totalDuration := time.Since(startTime)
		methodLogger.Error("Failed to decode Drupal response",
			logger.String("endpoint", endpoint),
			logger.String("article_title", req.Title),
			logger.Int("status_code", resp.StatusCode),
			logger.Duration("request_duration", requestDuration),
			logger.Duration("total_duration", totalDuration),
			logger.Error(err),
		)
		return fmt.Errorf("decode response: %w", err)
	}

	totalDuration := time.Since(startTime)
	methodLogger.Info("Successfully posted article to Drupal",
		logger.String("endpoint", endpoint),
		logger.String("article_title", req.Title),
		logger.String("content_type", req.ContentType),
		logger.String("drupal_id", drupalResp.Data.ID),
		logger.String("drupal_type", drupalResp.Data.Type),
		logger.Int("status_code", resp.StatusCode),
		logger.Duration("request_duration", requestDuration),
		logger.Duration("total_duration", totalDuration),
	)

	return nil
}
