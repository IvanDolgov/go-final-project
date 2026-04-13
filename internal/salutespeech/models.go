package salutespeech

import "time"

// TokenResponse ответ на запрос токена
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"` // unix timestamp in milliseconds
}

// UploadFileResponse ответ после загрузки файла
type UploadFileResponse struct {
	Result struct {
		RequestFileID string `json:"request_file_id"`
	} `json:"result"`
	Status int `json:"status"`
}

// AsyncRecognizeRequest тело запроса на создание задачи распознавания
type AsyncRecognizeRequest struct {
	Options struct {
		AudioEncoding string `json:"audio_encoding"` // например "MP3"
		Model         string `json:"model,omitempty"`
		Language      string `json:"language,omitempty"`
	} `json:"options"`
	RequestFileID string `json:"request_file_id"`
}

// AsyncRecognizeResponse ответ на создание задачи
type AsyncRecognizeResponse struct {
	Result struct {
		ID        string    `json:"id"` // task_id
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Status    string    `json:"status"` // "NEW"
	} `json:"result"`
	Status int `json:"status"`
}

// TaskStatusResponse ответ о статусе задачи
type TaskStatusResponse struct {
	Status int `json:"status"`
	Result struct {
		ID             string    `json:"id"`
		Status         string    `json:"status"` // "NEW", "RUNNING", "DONE", "ERROR", "CANCELED"
		CreatedAt      time.Time `json:"created_at"`
		UpdatedAt      time.Time `json:"updated_at"`
		ResponseFileID string    `json:"response_file_id,omitempty"` // ID файла с результатом
		Error          string    `json:"error,omitempty"`            // Описание ошибки
	} `json:"result"`
}

// RecognitionResultResponse ответ с результатом распознавания
type RecognitionResultResponse struct {
	// Содержит массив результатов, структура зависит от API
}
