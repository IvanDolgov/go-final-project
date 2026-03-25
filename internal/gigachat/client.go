package gigachat

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TokenManager struct {
	authKey     string
	scope       string
	tokenURL    string
	httpClient  *http.Client
	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
	logger      *zap.Logger
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model   string `json:"model"`
	Created int64  `json:"created"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
}

func NewTokenManager(authKey, scope, tokenURL string, insecureSkipVerify bool, logger *zap.Logger) *TokenManager {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
	}

	httpClient := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	return &TokenManager{
		authKey:    authKey,
		scope:      scope,
		tokenURL:   tokenURL,
		httpClient: httpClient,
		logger:     logger.With(zap.String("component", "gigachat_token_manager")),
	}
}

func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.RLock()
	if tm.accessToken != "" && time.Now().Add(1*time.Minute).Before(tm.expiresAt) {
		defer tm.mu.RUnlock()
		return tm.accessToken, nil
	}
	tm.mu.RUnlock()
	return tm.refreshToken()
}

func (tm *TokenManager) refreshToken() (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.accessToken != "" && time.Now().Add(1*time.Minute).Before(tm.expiresAt) {
		return tm.accessToken, nil
	}

	data := url.Values{}
	data.Set("scope", tm.scope)

	req, err := http.NewRequest("POST", tm.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+tm.authKey)
	req.Header.Set("RqUID", uuid.New().String())

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	tm.accessToken = tokenResp.AccessToken
	tm.expiresAt = time.Unix(0, tokenResp.ExpiresAt*int64(time.Millisecond))

	tm.logger.Info("GigaChat token refreshed", zap.Time("expires_at", tm.expiresAt))
	return tm.accessToken, nil
}

type Client struct {
	tokenManager *TokenManager
	apiURL       string
	httpClient   *http.Client
	logger       *zap.Logger
}

func NewClient(authKey, scope, tokenURL, apiURL string, insecureSkipVerify bool, logger *zap.Logger) *Client {
	tokenManager := NewTokenManager(authKey, scope, tokenURL, insecureSkipVerify, logger)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
	}

	httpClient := &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	return &Client{
		tokenManager: tokenManager,
		apiURL:       apiURL,
		httpClient:   httpClient,
		logger:       logger.With(zap.String("component", "gigachat_client")),
	}
}

func (c *Client) Chat(message string) (string, error) {
	token, err := c.tokenManager.GetToken()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	reqBody := ChatRequest{
		Model: "GigaChat",
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: message,
			},
		},
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.apiURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	c.logger.Info("GigaChat response received",
		zap.Int("tokens", chatResp.Usage.TotalTokens),
		zap.String("model", chatResp.Model),
	)

	return chatResp.Choices[0].Message.Content, nil
}

// GenerateSummary генерирует краткую выжимку текста
func (c *Client) GenerateSummary(text string) (string, error) {
	prompt := fmt.Sprintf("Сделай краткую выжимку этой встречи. Выдели основные тезисы, решения и договоренности.\n\nТекст встречи:\n%s", text)

	response, err := c.Chat(prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	return response, nil
}
