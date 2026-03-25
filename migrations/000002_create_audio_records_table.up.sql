CREATE TABLE IF NOT EXISTS audio_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    telegram_file_id TEXT NOT NULL,
    salutespeech_file_id UUID,
    task_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    recognition_text TEXT,
    summary TEXT, 
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audio_records_user_id ON audio_records(user_id);
CREATE INDEX idx_audio_records_status ON audio_records(status);
CREATE INDEX idx_audio_records_task_id ON audio_records(task_id);