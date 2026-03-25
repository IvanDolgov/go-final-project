package salutespeech

import (
	"crypto/tls"
	"encoding/base64"
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
	clientID     string
	clientSecret string
	scope        string
	tokenURL     string
	httpClient   *http.Client
	mu           sync.RWMutex
	accessToken  string
	expiresAt    time.Time
	logger       *zap.Logger
}

func NewTokenManager(clientID, clientSecret, scope, tokenURL string, insecureSkipVerify bool, logger *zap.Logger) *TokenManager {
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
		clientID:     clientID,
		clientSecret: clientSecret,
		scope:        scope,
		tokenURL:     tokenURL,
		httpClient:   httpClient,
		logger:       logger.With(zap.String("component", "token_manager")),
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

	// Агрессивная очистка credentials
	cleanID := cleanCredential(tm.clientID)
	cleanSecret := cleanCredential(tm.clientSecret)

	// Для отладки - логируем длину
	tm.logger.Debug("Credential lengths",
		zap.Int("original_id_len", len(tm.clientID)),
		zap.Int("clean_id_len", len(cleanID)),
		zap.Int("original_secret_len", len(tm.clientSecret)),
		zap.Int("clean_secret_len", len(cleanSecret)),
	)

	authString := cleanID + ":" + cleanSecret
	auth := base64.StdEncoding.EncodeToString([]byte(authString))

	data := url.Values{}
	data.Set("scope", cleanCredential(tm.scope))

	req, err := http.NewRequest("POST", tm.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("RqUID", uuid.New().String())

	// Логируем первые несколько символов auth header для проверки
	tm.logger.Debug("Auth header preview",
		zap.String("auth_preview", "Basic "+auth[:min(20, len(auth))]+"..."),
	)

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed with status %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	tm.accessToken = tokenResp.AccessToken
	tm.expiresAt = time.Unix(0, tokenResp.ExpiresAt*int64(time.Millisecond))

	return tm.accessToken, nil
}

// cleanCredential удаляет все проблемные символы
func cleanCredential(s string) string {
	var result strings.Builder
	for _, r := range s {
		// Оставляем только печатаемые ASCII символы
		if r >= 32 && r <= 126 && r != '"' && r != '\'' && r != '`' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// Вспомогательная функция
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (tm *TokenManager) GetHTTPClient() *http.Client {
	return tm.httpClient
}
