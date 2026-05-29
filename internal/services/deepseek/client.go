package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/config"
)

// Message represents one chat message for DeepSeek-compatible chat completions.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompleter is the minimal dependency required by dehydration services.
type ChatCompleter interface {
	ChatCompletion(ctx context.Context, messages []Message, stream bool) (string, error)
}

type Client struct {
	HTTPClient            *http.Client
	APIKey                string
	Model                 string
	BaseURL               string
	MaxRetries            int
	pool                  *pgxpool.Pool
	sleep                 func(time.Duration)
	runtimeSettingsLoader func(context.Context) (runtimeSettings, error)
}

type runtimeSettings struct {
	APIKey  string
	Model   string
	BaseURL string
}

func mergeRuntimeSettings(base runtimeSettings, apiKey, model, baseURL string) runtimeSettings {
	dbKey := strings.TrimSpace(apiKey)
	if dbKey != "" {
		base.APIKey = dbKey
	}
	if dbModel := strings.TrimSpace(model); dbModel != "" {
		base.Model = dbModel
	}
	if dbBaseURL := strings.TrimSpace(baseURL); dbBaseURL != "" {
		base.BaseURL = dbBaseURL
	}
	return base
}

func NewClient(pool *pgxpool.Pool, cfg *config.Config) *Client {
	client := &Client{
		HTTPClient: &http.Client{Timeout: 5 * time.Minute},
		pool:       pool,
		sleep:      time.Sleep,
	}
	if cfg != nil {
		client.APIKey = cfg.DeepseekAPIKey
		client.Model = cfg.DeepseekModel
		client.BaseURL = cfg.DeepseekBaseURL
		client.MaxRetries = cfg.MaxRetries
	}
	if client.Model == "" {
		client.Model = "deepseek-v4-flash"
	}
	if client.BaseURL == "" {
		client.BaseURL = "https://api.deepseek.com"
	}
	if client.MaxRetries <= 0 {
		client.MaxRetries = 1
	}
	client.runtimeSettingsLoader = client.loadRuntimeSettings
	return client
}

func (c *Client) ChatCompletion(ctx context.Context, messages []Message, stream bool) (string, error) {
	settings, err := c.runtimeSettingsLoader(ctx)
	if err != nil {
		return "", fmt.Errorf("deepseek: load runtime settings: %w", err)
	}

	payloadBytes, err := json.Marshal(map[string]any{
		"model":       settings.Model,
		"messages":    messages,
		"stream":      stream,
		"temperature": 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("deepseek: marshal payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < c.MaxRetries; attempt++ {
		result, retry, err := c.doRequest(ctx, settings, payloadBytes, stream, attempt)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}

	return "", fmt.Errorf("deepseek: request failed after %d retries: %w", c.MaxRetries, lastErr)
}

func (c *Client) doRequest(ctx context.Context, settings runtimeSettings, payload []byte, stream bool, attempt int) (string, bool, error) {
	endpoint := strings.TrimRight(settings.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		c.backoff(attempt, false)
		return "", true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		c.backoff(attempt, true)
		return "", true, fmt.Errorf("status 429: %s", strings.TrimSpace(string(body)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		c.backoff(attempt, false)
		return "", true, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if stream {
		result, err := parseStream(resp.Body)
		return result, false, err
	}
	result, err := parseResponse(resp.Body)
	return result, false, err
}

func (c *Client) loadRuntimeSettings(ctx context.Context) (runtimeSettings, error) {
	settings := runtimeSettings{
		APIKey:  strings.TrimSpace(c.APIKey),
		Model:   strings.TrimSpace(c.Model),
		BaseURL: strings.TrimSpace(c.BaseURL),
	}
	if c.pool == nil {
		return settings, nil
	}

	var apiKey, model, baseURL string
	err := c.pool.QueryRow(ctx, `SELECT deepseek_api_key, deepseek_model, deepseek_base_url FROM app_settings WHERE id=1`).Scan(&apiKey, &model, &baseURL)
	if err != nil {
		return settings, nil
	}
	return mergeRuntimeSettings(settings, apiKey, model, baseURL), nil
}

func (c *Client) backoff(attempt int, rateLimited bool) {
	if c.sleep == nil {
		return
	}
	if rateLimited {
		c.sleep(30 * time.Second * time.Duration(attempt+1))
		return
	}
	c.sleep(time.Second * time.Duration(1<<attempt))
}

func parseResponse(body io.Reader) (string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("decode response: missing choices")
	}
	return response.Choices[0].Message.Content, nil
}

func parseStream(body io.Reader) (string, error) {
	var builder strings.Builder
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var response struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			continue
		}
		if len(response.Choices) > 0 {
			builder.WriteString(response.Choices[0].Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read stream: %w", err)
	}
	return builder.String(), nil
}
