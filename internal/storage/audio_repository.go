package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/IvanDolgov/go-final-project/internal/models"
	"go.uber.org/zap"
)

type AudioRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewAudioRepository(db *sql.DB, logger *zap.Logger) *AudioRepository {
	return &AudioRepository{
		db:     db,
		logger: logger.With(zap.String("repository", "audio")),
	}
}

// Create создает новую запись об аудио
func (r *AudioRepository) Create(userID int64, telegramFileID string) (*models.AudioRecord, error) {
	query := `
		INSERT INTO audio_records (user_id, telegram_file_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, user_id, telegram_file_id, salutespeech_file_id, task_id, status, recognition_text, summary, created_at, updated_at
	`

	var record models.AudioRecord
	err := r.db.QueryRow(query, userID, telegramFileID, models.StatusPending).Scan(
		&record.ID, &record.UserID, &record.TelegramFileID,
		&record.SaluteSpeechFileID, &record.TaskID, &record.Status,
		&record.RecognitionText, &record.Summary, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		r.logger.Error("Failed to create audio record", zap.Error(err))
		return nil, fmt.Errorf("failed to create audio record: %w", err)
	}

	r.logger.Info("Audio record created", zap.Int64("record_id", record.ID), zap.Int64("user_id", userID))
	return &record, nil
}

// UpdateResult обновляет результат распознавания и summary
func (r *AudioRepository) UpdateResult(recordID int64, text string, summary string, status string) error {
	query := `
        UPDATE audio_records
        SET recognition_text = $1, summary = $2, status = $3, updated_at = NOW()
        WHERE id = $4
    `
	_, err := r.db.Exec(query, text, summary, status, recordID)
	if err != nil {
		r.logger.Error("Failed to update audio record result", zap.Error(err), zap.Int64("record_id", recordID))
		return fmt.Errorf("failed to update audio record result: %w", err)
	}
	r.logger.Info("Audio record result updated", zap.Int64("record_id", recordID), zap.String("status", status))
	return nil
}

// UpdateSummary обновляет только summary
func (r *AudioRepository) UpdateSummary(recordID int64, summary string) error {
	query := `
		UPDATE audio_records
		SET summary = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.Exec(query, summary, recordID)
	if err != nil {
		r.logger.Error("Failed to update summary", zap.Error(err), zap.Int64("record_id", recordID))
		return fmt.Errorf("failed to update summary: %w", err)
	}
	r.logger.Info("Summary updated", zap.Int64("record_id", recordID))
	return nil
}

// GetRecordByID возвращает запись по ID
func (r *AudioRepository) GetRecordByID(recordID int64) (*models.AudioRecord, error) {
	query := `
        SELECT id, user_id, telegram_file_id, salutespeech_file_id, task_id, 
               status, recognition_text, summary, created_at, updated_at
        FROM audio_records
        WHERE id = $1
    `

	var rec models.AudioRecord
	err := r.db.QueryRow(query, recordID).Scan(
		&rec.ID, &rec.UserID, &rec.TelegramFileID,
		&rec.SaluteSpeechFileID, &rec.TaskID, &rec.Status,
		&rec.RecognitionText, &rec.Summary, &rec.CreatedAt, &rec.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		r.logger.Error("Failed to get record by ID", zap.Error(err), zap.Int64("record_id", recordID))
		return nil, fmt.Errorf("failed to get record: %w", err)
	}

	return &rec, nil
}

// GetUserRecords возвращает все записи пользователя (обновляем сканирование)
func (r *AudioRepository) GetUserRecords(userID int64) ([]models.AudioRecord, error) {
	query := `
		SELECT id, user_id, telegram_file_id, salutespeech_file_id, task_id, 
		       status, recognition_text, summary, created_at, updated_at
		FROM audio_records
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		r.logger.Error("Failed to get user records", zap.Error(err), zap.Int64("user_id", userID))
		return nil, fmt.Errorf("failed to get user records: %w", err)
	}
	defer rows.Close()

	var records []models.AudioRecord
	for rows.Next() {
		var rec models.AudioRecord
		err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.TelegramFileID,
			&rec.SaluteSpeechFileID, &rec.TaskID, &rec.Status,
			&rec.RecognitionText, &rec.Summary, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err != nil {
			r.logger.Error("Failed to scan audio record", zap.Error(err))
			continue
		}
		records = append(records, rec)
	}

	return records, nil
}

// UpdateAfterUpload обновляет запись после загрузки файла в SaluteSpeech
func (r *AudioRepository) UpdateAfterUpload(recordID int64, fileID string) error {
	query := `
		UPDATE audio_records
		SET salutespeech_file_id = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.Exec(query, fileID, recordID)
	if err != nil {
		r.logger.Error("Failed to update audio record after upload", zap.Error(err), zap.Int64("record_id", recordID))
		return fmt.Errorf("failed to update audio record: %w", err)
	}
	r.logger.Debug("Audio record updated with file_id", zap.Int64("record_id", recordID), zap.String("file_id", fileID))
	return nil
}

// UpdateAfterTaskCreated обновляет запись после создания задачи
func (r *AudioRepository) UpdateAfterTaskCreated(recordID int64, taskID string) error {
	query := `
		UPDATE audio_records
		SET task_id = $1, status = $2, updated_at = NOW()
		WHERE id = $3
	`
	_, err := r.db.Exec(query, taskID, models.StatusProcessing, recordID)
	if err != nil {
		r.logger.Error("Failed to update audio record after task creation", zap.Error(err), zap.Int64("record_id", recordID))
		return fmt.Errorf("failed to update audio record: %w", err)
	}
	r.logger.Debug("Audio record updated with task_id", zap.Int64("record_id", recordID), zap.String("task_id", taskID))
	return nil
}

// GetPendingTasks возвращает записи со статусом processing (для опроса)
func (r *AudioRepository) GetProcessingTasks() ([]models.AudioRecord, error) {
	query := `
		SELECT id, user_id, telegram_file_id, salutespeech_file_id, task_id, status, recognition_text, created_at, updated_at
		FROM audio_records
		WHERE status = $1 AND task_id IS NOT NULL
	`
	rows, err := r.db.Query(query, models.StatusProcessing)
	if err != nil {
		r.logger.Error("Failed to get processing tasks", zap.Error(err))
		return nil, fmt.Errorf("failed to get processing tasks: %w", err)
	}
	defer rows.Close()

	var records []models.AudioRecord
	for rows.Next() {
		var rec models.AudioRecord
		err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.TelegramFileID,
			&rec.SaluteSpeechFileID, &rec.TaskID, &rec.Status,
			&rec.RecognitionText, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err != nil {
			r.logger.Error("Failed to scan audio record", zap.Error(err))
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// GetProcessingTasksWithTimeout возвращает записи со статусом processing, которые не превысили таймаут
func (r *AudioRepository) GetProcessingTasksWithTimeout(maxAge time.Duration) ([]models.AudioRecord, error) {
	// Конвертируем duration в минуты для SQL
	minutes := int(maxAge.Minutes())

	query := `
		SELECT id, user_id, telegram_file_id, salutespeech_file_id, task_id, 
		       status, recognition_text, created_at, updated_at
		FROM audio_records
		WHERE status = $1 AND task_id IS NOT NULL AND created_at > NOW() - ($2 || ' minutes')::interval
		ORDER BY created_at ASC
	`

	rows, err := r.db.Query(query, models.StatusProcessing, minutes)
	if err != nil {
		r.logger.Error("Failed to get processing tasks",
			zap.Error(err),
			zap.Int("minutes", minutes),
		)
		return nil, fmt.Errorf("failed to get processing tasks: %w", err)
	}
	defer rows.Close()

	var records []models.AudioRecord
	for rows.Next() {
		var rec models.AudioRecord
		err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.TelegramFileID,
			&rec.SaluteSpeechFileID, &rec.TaskID, &rec.Status,
			&rec.RecognitionText, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err != nil {
			r.logger.Error("Failed to scan audio record", zap.Error(err))
			continue
		}
		records = append(records, rec)
	}

	r.logger.Debug("Processing tasks retrieved",
		zap.Int("count", len(records)),
		zap.Int("minutes", minutes),
	)

	return records, nil
}

// GetExpiredTasks возвращает записи со статусом processing, которые превысили таймаут
func (r *AudioRepository) GetExpiredTasks(maxAge time.Duration) ([]models.AudioRecord, error) {
	// Конвертируем duration в минуты для SQL
	minutes := int(maxAge.Minutes())

	query := `
		SELECT id, user_id, telegram_file_id, salutespeech_file_id, task_id, 
		       status, recognition_text, created_at, updated_at
		FROM audio_records
		WHERE status = $1 AND task_id IS NOT NULL AND created_at <= NOW() - ($2 || ' minutes')::interval
		ORDER BY created_at ASC
	`

	rows, err := r.db.Query(query, models.StatusProcessing, minutes)
	if err != nil {
		r.logger.Error("Failed to get expired tasks",
			zap.Error(err),
			zap.Int("minutes", minutes),
		)
		return nil, fmt.Errorf("failed to get expired tasks: %w", err)
	}
	defer rows.Close()

	var records []models.AudioRecord
	for rows.Next() {
		var rec models.AudioRecord
		err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.TelegramFileID,
			&rec.SaluteSpeechFileID, &rec.TaskID, &rec.Status,
			&rec.RecognitionText, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err != nil {
			r.logger.Error("Failed to scan audio record", zap.Error(err))
			continue
		}
		records = append(records, rec)
	}

	if len(records) > 0 {
		r.logger.Info("Expired tasks found",
			zap.Int("count", len(records)),
			zap.Int("minutes", minutes),
		)
	}

	return records, nil
}
