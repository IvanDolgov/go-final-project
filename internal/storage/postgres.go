package storage

import (
	"database/sql"
	"fmt"

	"github.com/IvanDolgov/go-final-project/internal/config"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// NewPostgresConnection создает подключение к PostgreSQL
func NewPostgresConnection(cfg *config.Config, logger *zap.Logger) (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
	)

	logger.Debug("Connecting to database",
		zap.String("host", cfg.DBHost),
		zap.Int("port", cfg.DBPort),
		zap.String("database", cfg.DBName),
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		logger.Error("Failed to open database connection", zap.Error(err))
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Настраиваем пул соединений
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		logger.Error("Failed to ping database", zap.Error(err))
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Successfully connected to database")
	return db, nil
}
