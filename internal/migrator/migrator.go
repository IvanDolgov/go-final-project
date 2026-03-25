package migrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/zap"
)

// RunMigrations выполняет миграции базы данных
func RunMigrations(dbURL, migrationsPath string, logger *zap.Logger) error {
	logger.Info("Starting database migrations", zap.String("path", migrationsPath))

	// Проверяем существование директории с миграциями
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		return fmt.Errorf("migrations directory does not exist: %s", migrationsPath)
	}

	// Получаем абсолютный путь
	absPath, err := filepath.Abs(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	sourceURL := "file://" + absPath
	logger.Debug("Migration source", zap.String("url", sourceURL))

	// Создаем экземпляр мигратора
	m, err := migrate.New(sourceURL, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer m.Close()

	// Выполняем миграции
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("No new migrations to apply")
			return nil
		}
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	logger.Info("Migrations applied successfully")
	return nil
}
