package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/IvanDolgov/go-final-project/internal/models"
	"go.uber.org/zap"
)

type UserRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewUserRepository(db *sql.DB, logger *zap.Logger) *UserRepository {
	return &UserRepository{
		db:     db,
		logger: logger.With(zap.String("repository", "user")),
	}
}

// SaveUser сохраняет пользователя, если его еще нет в БД
func (r *UserRepository) SaveUser(telegramID int64) error {
	logger := r.logger.With(zap.Int64("telegram_id", telegramID))

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE telegram_id = $1)`
	err := r.db.QueryRow(query, telegramID).Scan(&exists)
	if err != nil {
		logger.Error("Failed to check user existence", zap.Error(err))
		return fmt.Errorf("failed to check user existence: %w", err)
	}

	if exists {
		logger.Debug("User already exists")
		return nil
	}

	insertQuery := `INSERT INTO users (telegram_id, created_at) VALUES ($1, $2)`
	_, err = r.db.Exec(insertQuery, telegramID, time.Now())
	if err != nil {
		logger.Error("Failed to insert user", zap.Error(err))
		return fmt.Errorf("failed to insert user: %w", err)
	}

	logger.Info("New user saved to database")
	return nil
}

// GetUserByTelegramID получает пользователя по telegram_id
func (r *UserRepository) GetUserByTelegramID(telegramID int64) (*models.User, error) {
	var user models.User
	query := `SELECT id, telegram_id, created_at FROM users WHERE telegram_id = $1`

	err := r.db.QueryRow(query, telegramID).Scan(&user.ID, &user.TelegramID, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Debug("User not found", zap.Int64("telegram_id", telegramID))
			return nil, nil
		}
		r.logger.Error("Failed to get user", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	r.logger.Debug("User retrieved", zap.Int64("telegram_id", telegramID), zap.Int64("id", user.ID))
	return &user, nil
}

// GetOrCreateUser возвращает внутренний ID пользователя по telegram_id, создавая запись при необходимости
func (r *UserRepository) GetOrCreateUser(telegramID int64) (int64, error) {
	var userID int64
	query := `SELECT id FROM users WHERE telegram_id = $1`
	err := r.db.QueryRow(query, telegramID).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if err != sql.ErrNoRows {
		r.logger.Error("Failed to get user by telegram_id", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return 0, fmt.Errorf("failed to get user: %w", err)
	}

	// Пользователь не найден — создаём
	insertQuery := `INSERT INTO users (telegram_id, created_at) VALUES ($1, NOW()) RETURNING id`
	err = r.db.QueryRow(insertQuery, telegramID).Scan(&userID)
	if err != nil {
		r.logger.Error("Failed to create user", zap.Int64("telegram_id", telegramID), zap.Error(err))
		return 0, fmt.Errorf("failed to create user: %w", err)
	}
	r.logger.Info("New user created via GetOrCreate", zap.Int64("telegram_id", telegramID), zap.Int64("user_id", userID))
	return userID, nil
}

// GetUserByID получает пользователя по внутреннему ID
func (r *UserRepository) GetUserByID(userID int64) (*models.User, error) {
	var user models.User
	query := `SELECT id, telegram_id, created_at FROM users WHERE id = $1`

	err := r.db.QueryRow(query, userID).Scan(&user.ID, &user.TelegramID, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			r.logger.Debug("User not found", zap.Int64("user_id", userID))
			return nil, nil
		}
		r.logger.Error("Failed to get user by ID", zap.Int64("user_id", userID), zap.Error(err))
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	r.logger.Debug("User retrieved by ID", zap.Int64("user_id", userID), zap.Int64("telegram_id", user.TelegramID))
	return &user, nil
}
