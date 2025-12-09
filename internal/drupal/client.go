package drupal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	token   string
	client  *http.Client
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
			Title    string `json:"title"`
			Body     string `json:"body"`
			FieldURL string `json:"field_url"`
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

func NewClient(baseURL, token string, skipTLSVerify bool) (*Client, error) {
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
	}

	return &Client{
		baseURL: baseURL,
		token:   token,
		client:  client,
	}, nil
}

func (c *Client) PostArticle(ctx context.Context, req ArticleRequest) error {
	drupalArticle := DrupalArticle{}
	drupalArticle.Data.Type = req.ContentType
	drupalArticle.Data.Attributes.Title = req.Title
	drupalArticle.Data.Attributes.Body = req.Body
	drupalArticle.Data.Attributes.FieldURL = req.URL
	drupalArticle.Data.Relationships.FieldGroup.Data.Type = req.GroupType
	drupalArticle.Data.Relationships.FieldGroup.Data.ID = req.GroupID

	payload, err := json.Marshal(drupalArticle)
	if err != nil {
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/vnd.api+json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var drupalResp DrupalResponse
		if err := json.NewDecoder(resp.Body).Decode(&drupalResp); err == nil && len(drupalResp.Errors) > 0 {
			return fmt.Errorf("drupal API error (%d): %s - %s",
				resp.StatusCode,
				drupalResp.Errors[0].Title,
				drupalResp.Errors[0].Detail)
		}
		return fmt.Errorf("drupal API error: %d %s", resp.StatusCode, resp.Status)
	}

	var drupalResp DrupalResponse
	if err := json.NewDecoder(resp.Body).Decode(&drupalResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
