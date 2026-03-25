package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IvanDolgov/go-final-project/internal/bot"
	"github.com/IvanDolgov/go-final-project/internal/config"
	"github.com/IvanDolgov/go-final-project/internal/gigachat"
	"github.com/IvanDolgov/go-final-project/internal/logger"
	"github.com/IvanDolgov/go-final-project/internal/migrator"
	"github.com/IvanDolgov/go-final-project/internal/salutespeech"
	"github.com/IvanDolgov/go-final-project/internal/storage"
	"github.com/IvanDolgov/go-final-project/internal/worker"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	logger.Init(cfg.Environment)
	log := logger.Get()
	defer logger.Sync()

	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN is required")
	}
	if cfg.SaluteSpeechClientID == "" || cfg.SaluteSpeechClientSecret == "" {
		log.Fatal("SaluteSpeech credentials are required")
	}

	// Миграции
	log.Info("Running database migrations")
	if err := migrator.RunMigrations(cfg.DSN(), cfg.MigrationsPath, log); err != nil {
		log.Fatal("Failed to run migrations", zap.Error(err))
	}

	// Подключение к БД
	log.Info("Connecting to database")
	db, err := storage.NewPostgresConnection(cfg, log)
	if err != nil {
		log.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Репозитории
	userRepo := storage.NewUserRepository(db, log)
	audioRepo := storage.NewAudioRepository(db, log)

	// HTTP клиент для SaluteSpeech
	httpClient := cfg.NewSaluteSpeechHTTPClient(30 * time.Second)

	// Инициализация SaluteSpeech клиента
	tokenManager := salutespeech.NewTokenManager(
		cfg.SaluteSpeechClientID,
		cfg.SaluteSpeechClientSecret,
		cfg.SaluteSpeechScope,
		cfg.SaluteSpeechTokenURL,
		cfg.SaluteSpeechInsecureSkipVerify,
		log,
	)
	saluteClient := salutespeech.NewClient(tokenManager, cfg.SaluteSpeechBaseURL, httpClient, log)

	// Создание бота
	log.Info("Initializing bot")
	b, err := bot.NewBot(cfg, log)
	if err != nil {
		log.Fatal("Failed to create bot", zap.Error(err))
	}

	// Инициализация GigaChat клиента (объявляем переменную)
	var gigaClient *gigachat.Client
	if cfg.GigaChatAuthKey != "" {
		log.Info("Initializing GigaChat client")
		gigaClient = gigachat.NewClient(
			cfg.GigaChatAuthKey,
			cfg.GigaChatScope,
			cfg.GigaChatTokenURL,
			cfg.GigaChatAPIURL,
			cfg.SaluteSpeechInsecureSkipVerify,
			log,
		)
	} else {
		log.Warn("GigaChat auth key not provided, summary generation will be unavailable")
	}

	// Создаем процессор аудио (передаем gigaClient)
	audioProcessor := worker.NewAudioProcessor(
		audioRepo,
		userRepo,
		saluteClient,
		gigaClient, // может быть nil
		b,
		log,
		3,
	)
	audioProcessor.Start()
	defer audioProcessor.Stop()

	// Регистрация обработчиков (передаем gigaClient)
	handlers := bot.NewBotHandlers(userRepo, audioRepo, saluteClient, gigaClient, log, audioProcessor)
	handlers.Register(b)

	// Запуск бота
	go func() {
		log.Info("Bot is starting...")
		b.Start()
	}()

	log.Info("Bot is running. Press Ctrl+C to stop.")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Bot is shutting down...")
	b.Stop()
	log.Info("Bot stopped")
}
