package salutespeech

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"go.uber.org/zap"
)

type Client struct {
	tokenManager *TokenManager
	baseURL      string
	httpClient   *http.Client
	logger       *zap.Logger
}

func NewClient(tokenManager *TokenManager, baseURL string, httpClient *http.Client, logger *zap.Logger) *Client {
	return &Client{
		tokenManager: tokenManager,
		baseURL:      baseURL,
		httpClient:   httpClient,
		logger:       logger.With(zap.String("component", "salutespeech_client")),
	}
}

// UploadFile загружает аудиофайл в хранилище SaluteSpeech
func (c *Client) UploadFile(filename string, fileData []byte) (string, error) {
	token, err := c.tokenManager.GetToken()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	_, err = part.Write(fileData)
	if err != nil {
		return "", fmt.Errorf("failed to write file data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", c.baseURL+"/data:upload", body)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed with status %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	var uploadResp UploadFileResponse
	if err := json.Unmarshal(bodyBytes, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	c.logger.Info("File uploaded",
		zap.String("file_id", uploadResp.Result.RequestFileID),
		zap.String("filename", filename),
		zap.Int("size", len(fileData)),
	)

	return uploadResp.Result.RequestFileID, nil
}

// CreateTask создает задачу на асинхронное распознавание
func (c *Client) CreateTask(fileID string, audioEncoding string) (string, error) {
	token, err := c.tokenManager.GetToken()
	if err != nil {
		return "", err
	}

	reqBody := map[string]interface{}{
		"request_file_id": fileID,
		"options": map[string]interface{}{
			"audio_encoding": audioEncoding,
			"model":          "general",
			"language":       "ru-RU",
			// Duration поля должны быть строками с суффиксом "s"
			"no_speech_timeout":       "2s",
			"max_speech_timeout":      "20s",
			"hypotheses_count":        1,
			"enable_profanity_filter": false,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	c.logger.Debug("Create task request",
		zap.String("body", string(jsonBody)),
	)

	req, err := http.NewRequest("POST", c.baseURL+"/speech:async_recognize", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create task failed with status %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	var taskResp AsyncRecognizeResponse
	if err := json.Unmarshal(bodyBytes, &taskResp); err != nil {
		return "", fmt.Errorf("failed to decode task response: %w", err)
	}

	c.logger.Info("Task created",
		zap.String("task_id", taskResp.Result.ID),
		zap.String("encoding", audioEncoding),
	)

	return taskResp.Result.ID, nil
}

// GetTaskStatus получает статус задачи
func (c *Client) GetTaskStatus(taskID string) (*TaskStatusResponse, error) {
	token, err := c.tokenManager.GetToken()
	if err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/task:get?id=%s", c.baseURL, taskID)

	c.logger.Debug("Getting task status",
		zap.String("url", reqURL),
		zap.String("task_id", taskID),
	)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("Task status response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(bodyBytes)),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get task status failed with status %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	var statusResp TaskStatusResponse
	if err := json.Unmarshal(bodyBytes, &statusResp); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	c.logger.Info("Task status retrieved",
		zap.String("task_id", taskID),
		zap.String("status", statusResp.Result.Status),
	)

	return &statusResp, nil
}

// GetRecognitionResult получает результат распознавания по file_id
func (c *Client) GetRecognitionResult(fileID string) ([]byte, error) {
	token, err := c.tokenManager.GetToken()
	if err != nil {
		return nil, err
	}

	// ВАЖНО: используем response_file_id, а не request_file_id
	reqURL := fmt.Sprintf("%s/data:download?response_file_id=%s", c.baseURL, fileID)

	c.logger.Debug("Downloading recognition result",
		zap.String("url", reqURL),
		zap.String("file_id", fileID),
	)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("Failed to download result",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(bodyBytes)),
		)
		return nil, fmt.Errorf("download failed with status %d: %s",
			resp.StatusCode, string(bodyBytes))
	}

	c.logger.Info("Recognition result downloaded successfully",
		zap.String("file_id", fileID),
		zap.Int("size_bytes", len(bodyBytes)),
	)

	return bodyBytes, nil
}
