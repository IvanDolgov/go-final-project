package bot

import (
	"net/http"
	"net/url"
	"time"

	"github.com/IvanDolgov/go-final-project/internal/config"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
	tele "gopkg.in/telebot.v3"
)

// NewBot создаёт нового бота с кастомным HTTP-клиентом и поддержкой прокси
func NewBot(cfg *config.Config, logger *zap.Logger) (*tele.Bot, error) {
	// Настраиваем HTTP транспорт
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
	}

	// Если указан прокси, настраиваем
	if cfg.ProxyURL != "" {
		parsed, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			logger.Warn("Invalid proxy URL, ignoring", zap.Error(err))
		} else {
			switch parsed.Scheme {
			case "http", "https":
				transport = &http.Transport{
					Proxy:                 http.ProxyURL(parsed),
					TLSHandshakeTimeout:   30 * time.Second,
					ResponseHeaderTimeout: 30 * time.Second,
				}
			case "socks5":
				transport, err = socks5Transport(cfg.ProxyURL)
				if err != nil {
					logger.Warn("Failed to create SOCKS5 transport", zap.Error(err))
				}
			default:
				logger.Warn("Unsupported proxy scheme", zap.String("scheme", parsed.Scheme))
			}
		}
	}
	if transport == nil {
		// создаём обычный транспорт без прокси
		transport = &http.Transport{
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		}
	}

	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	pref := tele.Settings{
		Token:  cfg.BotToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
		Client: client,
		OnError: func(err error, c tele.Context) {
			logger.Error("Telegram bot error", zap.Error(err), zap.Any("context", c))
		},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		logger.Error("Failed to create bot", zap.Error(err))
		return nil, err
	}

	logger.Info("Bot initialized successfully")
	return b, nil
}

func socks5Transport(proxyURL string) (*http.Transport, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	dialer, err := proxy.FromURL(parsed, proxy.Direct)
	if err != nil {
		return nil, err
	}
	return &http.Transport{
		DialContext:           dialer.(proxy.ContextDialer).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}, nil
}
