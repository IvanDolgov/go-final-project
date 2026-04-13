package config

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken       string
	DBHost         string
	DBPort         int
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	Environment    string
	MigrationsPath string

	// Прокси для HTTP-клиентов (например, для Telegram API)
	ProxyURL string

	// SaluteSpeech
	SaluteSpeechClientID           string
	SaluteSpeechClientSecret       string
	SaluteSpeechScope              string
	SaluteSpeechTokenURL           string
	SaluteSpeechBaseURL            string
	SaluteSpeechInsecureSkipVerify bool

	// Gigachat
	GigaChatAuthKey  string
	GigaChatScope    string
	GigaChatTokenURL string
	GigaChatAPIURL   string
}

func Load() *Config {
	// Загружаем .env файл, если он существует
	_ = godotenv.Load() // игнорируем ошибку, если файла нет

	dbPort, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	migrationsPath := getEnv("MIGRATIONS_PATH", "./migrations")

	return &Config{
		BotToken:       getEnv("BOT_TOKEN", ""),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         dbPort,
		DBUser:         getEnv("DB_USER", "postgres"),
		DBPassword:     getEnv("DB_PASSWORD", "postgres"),
		DBName:         getEnv("DB_NAME", "audio_bot"),
		DBSSLMode:      getEnv("DB_SSLMODE", "disable"),
		Environment:    getEnv("ENVIRONMENT", "development"),
		MigrationsPath: migrationsPath,
		ProxyURL:       getEnv("PROXY_URL", ""),

		SaluteSpeechClientID:           getEnv("SALUTE_SPEECH_CLIENT_ID", ""),
		SaluteSpeechClientSecret:       getEnv("SALUTE_SPEECH_CLIENT_SECRET", ""),
		SaluteSpeechScope:              getEnv("SALUTE_SPEECH_SCOPE", "SALUTE_SPEECH_PERS"),
		SaluteSpeechTokenURL:           getEnv("SALUTE_SPEECH_TOKEN_URL", "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"),
		SaluteSpeechBaseURL:            getEnv("SALUTE_SPEECH_BASE_URL", "https://smartspeech.sber.ru/rest/v1"),
		SaluteSpeechInsecureSkipVerify: getEnv("SALUTE_SPEECH_INSECURE_SKIP_VERIFY", "false") == "true",

		// GigaChat
		GigaChatAuthKey:  getEnv("GIGACHAT_AUTH_KEY", ""),
		GigaChatScope:    getEnv("GIGACHAT_SCOPE", "GIGACHAT_API_PERS"),
		GigaChatTokenURL: getEnv("GIGACHAT_TOKEN_URL", "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"),
		GigaChatAPIURL:   getEnv("GIGACHAT_API_URL", "https://gigachat.devices.sberbank.ru/api/v1"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// DSN возвращает строку подключения к БД в формате для migrate
func (c *Config) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}

// NewSaluteSpeechHTTPClient создаёт HTTP-клиент с поддержкой прокси и InsecureSkipVerify
func (c *Config) NewSaluteSpeechHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
	}

	// Настройка TLS с InsecureSkipVerify
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: c.SaluteSpeechInsecureSkipVerify,
	}

	// Если указан прокси
	if c.ProxyURL != "" {
		if proxyURL, err := url.Parse(c.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
