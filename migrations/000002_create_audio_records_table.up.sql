CREATE TABLE IF NOT EXISTS audio_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    telegram_file_id TEXT NOT NULL,
    salutespeech_file_id UUID,
    task_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    raw_response TEXT,           -- полный ответ от SaluteSpeech (JSON)
    text TEXT,                   -- собранный текст из всех results.text
    normalized_text TEXT,        -- собранный нормализованный текст из всех results.normalized_text
    summary TEXT,                -- краткая выжимка от GigaChat
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audio_records_user_id ON audio_records(user_id);
CREATE INDEX idx_audio_records_status ON audio_records(status);
CREATE INDEX idx_audio_records_task_id ON audio_records(task_id);
-- Добавляем индекс для полнотекстового поиска
CREATE INDEX idx_audio_records_text_search ON audio_records 
  USING gin(to_tsvector('russian', COALESCE(normalized_text, text)));