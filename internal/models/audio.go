package models

import (
	"database/sql"
	"time"
)

type AudioRecord struct {
	ID                 int64          `db:"id"`
	UserID             int64          `db:"user_id"`
	TelegramFileID     string         `db:"telegram_file_id"`
	SaluteSpeechFileID sql.NullString `db:"salutespeech_file_id"`
	TaskID             sql.NullString `db:"task_id"`
	Status             string         `db:"status"`
	RawResponse        sql.NullString `db:"raw_response"`    // полный JSON ответ
	Text               sql.NullString `db:"text"`            // собранный text
	NormalizedText     sql.NullString `db:"normalized_text"` // собранный normalized_text
	Summary            sql.NullString `db:"summary"`         // выжимка от GigaChat
	CreatedAt          time.Time      `db:"created_at"`
	UpdatedAt          time.Time      `db:"updated_at"`
}

// Константы статусов
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)
