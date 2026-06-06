package chroma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, httpClient: &http.Client{}}
}

func (c *Client) Query(ctx context.Context, collection string, query string, limit int) ([]map[string]any, error) {
	payload, _ := json.Marshal(map[string]any{"query_texts": []string{query}, "n_results": limit})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/v1/collections/%s/query", c.baseURL, collection), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chroma query failed: %s", resp.Status)
	}
	return []map[string]any{}, nil
}
