package drupal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	Title         string
	Body          string
	URL           string
	GroupID       string
	GroupType     string
	ContentType   string
	ExternalID    string
	Intro         string
	Description   string
	OGTitle       string
	OGDescription string
	OGImage       string
	OGURL         string
	WordCount     int
	Category      string
	Section       string
	Keywords      []string
	CanonicalURL  string
	PublishedDate time.Time
}

type GroupReference struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Meta struct {
		DrupalInternalTargetID int `json:"drupal_internal__target_id,omitempty"`
	} `json:"meta,omitempty"`
}

type DrupalArticle struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Title              string         `json:"title"`
			Body               map[string]any `json:"body,omitempty"`
			FieldURL           map[string]any `json:"field_url,omitempty"`
			FieldExternalID    string         `json:"field_external_id,omitempty"`
			FieldIntro         string         `json:"field_intro,omitempty"`
			FieldDescription   string         `json:"field_description,omitempty"`
			FieldOGTitle       string         `json:"field_og_title,omitempty"`
			FieldOGDescription string         `json:"field_og_description,omitempty"`
			FieldOGImage       string         `json:"field_og_image,omitempty"`
			FieldOGURL         string         `json:"field_og_url,omitempty"`
			FieldWordCount     string         `json:"field_word_count,omitempty"`
			FieldCategory      string         `json:"field_category,omitempty"`
			FieldSection       string         `json:"field_section,omitempty"`
			FieldKeywords      string         `json:"field_keywords,omitempty"`
			FieldCanonicalURL  string         `json:"field_canonical_url,omitempty"`
			FieldPublishedDate string         `json:"field_published_date,omitempty"`
		} `json:"attributes"`
		Relationships struct {
			FieldGroup *struct {
				Data []GroupReference `json:"data"`
			} `json:"field_group,omitempty"`
		} `json:"relationships,omitempty"`
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
		return nil, errors.New("drupal URL is required")
	}
	if token == "" {
		return nil, errors.New("drupal token is required")
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

// setAuthHeaders sets the authentication headers required for Drupal REST API
// This includes API-KEY, Authorization, and AUTH-METHOD headers
func (c *Client) setAuthHeaders(req *http.Request) {
	// REST API Authentication module expects API-KEY header with base64(username:api-key)
	// Also include Authorization header with Basic format as miniOrange requires it
	var apiKeyValue string
	if c.username != "" {
		apiKeyValue = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.username, c.token)))
	} else {
		// Fallback: if no username, just use token (base64 encoded)
		apiKeyValue = base64.StdEncoding.EncodeToString([]byte(c.token))
	}

	req.Header.Set("API-KEY", apiKeyValue)
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", apiKeyValue))

	// Include AUTH-METHOD header if configured (required by miniOrange REST API Authentication)
	if c.authMethod != "" {
		req.Header.Set("AUTH-METHOD", c.authMethod)
	}
}

// getCSRFToken fetches a CSRF token from Drupal's session/token endpoint
// Note: The session/token endpoint may require Basic Auth, while JSON:API uses API-KEY header
func (c *Client) getCSRFToken(ctx context.Context) (string, error) {
	tokenURL := fmt.Sprintf("%s/session/token", c.baseURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create CSRF token request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	c.setAuthHeaders(httpReq)

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

// mapArticleFields maps ArticleRequest fields to DrupalArticle attributes
func (c *Client) mapArticleFields(req ArticleRequest, drupalArticle *DrupalArticle) {
	drupalArticle.Data.Type = req.ContentType
	drupalArticle.Data.Attributes.Title = req.Title

	if req.Body != "" {
		// Drupal body field requires value and format structure
		drupalArticle.Data.Attributes.Body = map[string]any{
			"value":  req.Body,
			"format": "full_html", // Use full_html format to preserve HTML content
		}
	}

	// field_url is required - send as URI field in attributes
	if req.URL != "" {
		drupalArticle.Data.Attributes.FieldURL = map[string]any{
			"uri": req.URL,
		}
	}

	// Map additional fields from Elasticsearch
	if req.ExternalID != "" {
		drupalArticle.Data.Attributes.FieldExternalID = req.ExternalID
	}
	if req.Intro != "" {
		drupalArticle.Data.Attributes.FieldIntro = req.Intro
	}
	if req.Description != "" {
		drupalArticle.Data.Attributes.FieldDescription = req.Description
	}
	if req.OGTitle != "" {
		drupalArticle.Data.Attributes.FieldOGTitle = req.OGTitle
	}
	if req.OGDescription != "" {
		drupalArticle.Data.Attributes.FieldOGDescription = req.OGDescription
	}
	if req.OGImage != "" {
		drupalArticle.Data.Attributes.FieldOGImage = req.OGImage
	}
	if req.OGURL != "" {
		drupalArticle.Data.Attributes.FieldOGURL = req.OGURL
	}
	if req.WordCount > 0 {
		// Drupal may expect word_count as string, convert to string
		drupalArticle.Data.Attributes.FieldWordCount = strconv.Itoa(req.WordCount)
	}
	if req.Category != "" {
		drupalArticle.Data.Attributes.FieldCategory = req.Category
	}
	if req.Section != "" {
		drupalArticle.Data.Attributes.FieldSection = req.Section
	}
	if len(req.Keywords) > 0 {
		// Join keywords array into a single string (pipe-separated as shown in ES)
		drupalArticle.Data.Attributes.FieldKeywords = strings.Join(req.Keywords, "|")
	}
	if req.CanonicalURL != "" {
		drupalArticle.Data.Attributes.FieldCanonicalURL = req.CanonicalURL
	}
	// field_published_date - convert time.Time to ISO8601 format
	if !req.PublishedDate.IsZero() {
		// Drupal expects ISO8601 format (e.g., "2025-12-09T00:00:00Z")
		drupalArticle.Data.Attributes.FieldPublishedDate = req.PublishedDate.Format(time.RFC3339)
	}
}

func (c *Client) PostArticle(ctx context.Context, req ArticleRequest) error {
	startTime := time.Now()

	// Add method-level context
	methodLogger := c.logger.With(
		logger.String("method", "PostArticle"),
	)

	drupalArticle := DrupalArticle{}
	c.mapArticleFields(req, &drupalArticle)

	// field_group is optional - only include if GroupID is provided
	// Drupal JSON:API expects relationship format with type and id (UUID)
	if req.GroupID != "" && req.GroupType != "" {
		groupItem := GroupReference{
			Type: req.GroupType,
			ID:   req.GroupID, // Use UUID, not numeric ID
		}

		// Include meta.drupal_internal__target_id if we know it
		// For UUID e3d024a6-5f6f-4be8-8f3d-75639075959c, the numeric ID is 1
		// TODO: Fetch group by UUID to get the numeric ID dynamically
		if req.GroupID == "e3d024a6-5f6f-4be8-8f3d-75639075959c" {
			groupItem.Meta.DrupalInternalTargetID = 1
		}

		drupalArticle.Data.Relationships.FieldGroup = &struct {
			Data []GroupReference `json:"data"`
		}{
			Data: []GroupReference{groupItem},
		}
	}

	payload, err := json.Marshal(drupalArticle)
	if err != nil {
		methodLogger.Error("Failed to marshal article payload",
			logger.String("title", req.Title),
			logger.String("content_type", req.ContentType),
			logger.Error(err),
		)
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Debug: Log the payload to verify group relationship
	methodLogger.Debug("Article payload prepared",
		logger.String("group_type", req.GroupType),
		logger.String("group_id", req.GroupID),
		logger.String("payload", string(payload)),
	)

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

	httpReq, httpErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(payload))
	if httpErr != nil {
		methodLogger.Error("Failed to create HTTP request",
			logger.String("endpoint", endpoint),
			logger.String("title", req.Title),
			logger.Error(httpErr),
		)
		return fmt.Errorf("create request: %w", httpErr)
	}

	httpReq.Header.Set("Content-Type", "application/vnd.api+json")
	httpReq.Header.Set("Accept", "application/vnd.api+json")
	c.setAuthHeaders(httpReq)

	// Fetch and include CSRF token for POST requests
	csrfToken, csrfErr := c.getCSRFToken(ctx)
	if csrfErr != nil {
		methodLogger.Warn("Failed to fetch CSRF token, proceeding without it",
			logger.String("endpoint", endpoint),
			logger.Error(csrfErr),
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

	const badRequestStatusCode = 400
	if resp.StatusCode >= badRequestStatusCode {
		// Read the full response body for debugging
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)

		var drupalResp DrupalResponse
		decodeErr := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&drupalResp)

		if decodeErr == nil && len(drupalResp.Errors) > 0 {
			// Log all validation errors, not just the first one
			errorDetails := make([]string, len(drupalResp.Errors))
			for i, err := range drupalResp.Errors {
				errorDetails[i] = fmt.Sprintf("%s: %s", err.Title, err.Detail)
			}
			allErrors := strings.Join(errorDetails, "; ")

			errorDetail := drupalResp.Errors[0]
			methodLogger.Error("Drupal API validation error",
				logger.String("endpoint", endpoint),
				logger.String("article_title", req.Title),
				logger.String("group_type", req.GroupType),
				logger.String("group_id", req.GroupID),
				logger.Int("status_code", resp.StatusCode),
				logger.String("status", resp.Status),
				logger.Int("error_count", len(drupalResp.Errors)),
				logger.String("all_errors", allErrors),
				logger.String("error_status", errorDetail.Status),
				logger.String("error_title", errorDetail.Title),
				logger.String("error_detail", errorDetail.Detail),
				logger.String("response_body", bodyStr),
				logger.Duration("request_duration", requestDuration),
			)
			return fmt.Errorf("drupal API error (%d): %s - %s",
				resp.StatusCode,
				errorDetail.Title,
				allErrors)
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
	if decodeErr := json.NewDecoder(resp.Body).Decode(&drupalResp); decodeErr != nil {
		totalDuration := time.Since(startTime)
		methodLogger.Error("Failed to decode Drupal response",
			logger.String("endpoint", endpoint),
			logger.String("article_title", req.Title),
			logger.Int("status_code", resp.StatusCode),
			logger.Duration("request_duration", requestDuration),
			logger.Duration("total_duration", totalDuration),
			logger.Error(decodeErr),
		)
		return fmt.Errorf("decode response: %w", decodeErr)
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

// doJSONAPIRequest performs a GET request to a Drupal JSON:API endpoint and returns the parsed response
func (c *Client) doJSONAPIRequest(ctx context.Context, endpoint string) (map[string]any, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.api+json")
	c.setAuthHeaders(httpReq)

	resp, respErr := c.client.Do(httpReq)
	if respErr != nil {
		return nil, fmt.Errorf("http request: %w", respErr)
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("read response: %w", readErr)
	}

	const badRequestStatusCode = 400
	if resp.StatusCode >= badRequestStatusCode {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]any
	if decodeErr := json.Unmarshal(bodyBytes, &result); decodeErr != nil {
		return nil, fmt.Errorf("decode response: %w", decodeErr)
	}

	return result, nil
}

// GetNode fetches a node by ID from Drupal JSON:API (temporary method for debugging)
// nodeID can be either a UUID or numeric ID
func (c *Client) GetNode(ctx context.Context, nodeID string) (map[string]any, error) {
	endpoint := fmt.Sprintf("%s/jsonapi/node/article/%s", c.baseURL, nodeID)
	return c.doJSONAPIRequest(ctx, endpoint)
}

// ListNodes lists articles from Drupal JSON:API (temporary method for debugging)
func (c *Client) ListNodes(ctx context.Context, limit int) (map[string]any, error) {
	endpoint := fmt.Sprintf("%s/jsonapi/node/article?page[limit]=%d", c.baseURL, limit)
	return c.doJSONAPIRequest(ctx, endpoint)
}
