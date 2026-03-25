package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/IvanDolgov/go-final-project/internal/gigachat"
	"github.com/IvanDolgov/go-final-project/internal/models"
	"github.com/IvanDolgov/go-final-project/internal/queue"
	"github.com/IvanDolgov/go-final-project/internal/salutespeech"
	"github.com/IvanDolgov/go-final-project/internal/storage"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

const (
	maxWaitTime     = 20 * time.Minute
	pollingInterval = 10 * time.Second
)

type AudioProcessor struct {
	audioRepo    *storage.AudioRepository
	userRepo     *storage.UserRepository
	saluteClient *salutespeech.Client
	gigaClient   *gigachat.Client
	bot          *tele.Bot
	logger       *zap.Logger
	jobQueue     *queue.JobQueue
	resultQueue  *queue.ResultQueue
	workerCount  int
	stopChan     chan struct{}
}

func NewAudioProcessor(
	audioRepo *storage.AudioRepository,
	userRepo *storage.UserRepository,
	client *salutespeech.Client,
	gigaClient *gigachat.Client,
	bot *tele.Bot,
	logger *zap.Logger,
	workerCount int,
) *AudioProcessor {
	return &AudioProcessor{
		audioRepo:    audioRepo,
		userRepo:     userRepo,
		saluteClient: client,
		gigaClient:   gigaClient,
		bot:          bot,
		logger:       logger.With(zap.String("component", "audio_processor")),
		jobQueue:     queue.NewJobQueue(100),
		resultQueue:  queue.NewResultQueue(100),
		workerCount:  workerCount,
		stopChan:     make(chan struct{}),
	}
}

// Start запускает воркеров для обработки задач
func (p *AudioProcessor) Start() {
	// Запускаем воркеров для обработки задач
	for i := 0; i < p.workerCount; i++ {
		go p.worker(i)
	}

	// Запускаем отправитель результатов
	go p.resultSender()

	// Запускаем опрос статусов задач
	go p.statusPoller()

	p.logger.Info("Audio processor started",
		zap.Int("worker_count", p.workerCount),
		zap.Int("job_queue_size", cap(p.jobQueue.Jobs)),
	)
}

// Stop останавливает процессор
func (p *AudioProcessor) Stop() {
	close(p.stopChan)
	p.jobQueue.Close()
	p.resultQueue.Close()
	p.logger.Info("Audio processor stopped")
}

// SubmitJob отправляет задачу на обработку
func (p *AudioProcessor) SubmitJob(job queue.Job) {
	p.logger.Debug("Job submitted",
		zap.String("job_id", job.ID),
		zap.Int64("user_id", job.UserID),
	)
	p.jobQueue.Push(job)
}

// worker обрабатывает задачи из очереди
func (p *AudioProcessor) worker(workerID int) {
	logger := p.logger.With(zap.Int("worker_id", workerID))
	logger.Info("Worker started")

	for {
		select {
		case <-p.stopChan:
			logger.Info("Worker stopped")
			return
		case job := <-p.jobQueue.Jobs:
			logger.Debug("Processing job",
				zap.String("job_id", job.ID),
				zap.Int64("user_id", job.UserID),
			)

			// Обрабатываем задачу
			result := p.processJob(job)

			// Отправляем результат
			p.resultQueue.PushResult(result)
		}
	}
}

// processJob обрабатывает одну задачу
func (p *AudioProcessor) processJob(job queue.Job) queue.Result {
	logger := p.logger.With(
		zap.String("job_id", job.ID),
		zap.Int64("user_id", job.UserID),
	)

	result := queue.Result{
		JobID:  job.ID,
		UserID: job.UserID,
	}

	// Создаём запись в БД
	record, err := p.audioRepo.Create(job.UserDBID, job.FileID)
	if err != nil {
		logger.Error("Failed to create audio record", zap.Error(err))
		result.Success = false
		result.Error = "Ошибка сохранения информации о файле"
		result.Message = "❌ Произошла ошибка при сохранении информации о файле."
		return result
	}

	result.RecordID = record.ID

	// Отправляем промежуточное сообщение пользователю (через result queue)
	intermediateResult := queue.Result{
		JobID:   job.ID,
		UserID:  job.UserID,
		Success: true,
		Message: "✅ Файл получен, начинаю загрузку в сервис распознавания...",
	}
	p.resultQueue.PushResult(intermediateResult)

	// Скачиваем файл
	file, err := job.Chat.Bot().File(&tele.File{FileID: job.FileID})
	if err != nil {
		logger.Error("Failed to get file from Telegram", zap.Error(err))
		p.audioRepo.UpdateResult(record.ID, "Ошибка скачивания файла", "", "", models.StatusFailed)
		result.Success = false
		result.Error = "Ошибка скачивания файла"
		result.Message = "❌ Не удалось скачать аудиофайл."
		return result
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		logger.Error("Failed to read file data", zap.Error(err))
		p.audioRepo.UpdateResult(record.ID, "Ошибка чтения файла", "", "", models.StatusFailed)
		result.Success = false
		result.Error = "Ошибка чтения файла"
		result.Message = "❌ Ошибка при чтении файла."
		return result
	}

	// Загружаем в SaluteSpeech
	saluteFileID, err := p.saluteClient.UploadFile(job.FileName, fileData)
	if err != nil {
		logger.Error("Failed to upload file to SaluteSpeech", zap.Error(err))
		p.audioRepo.UpdateResult(record.ID, "Ошибка загрузки в сервис распознавания", "", "", models.StatusFailed)
		result.Success = false
		result.Error = "Ошибка загрузки в сервис распознавания"
		result.Message = "❌ Не удалось загрузить файл в сервис распознавания."
		return result
	}

	p.audioRepo.UpdateAfterUpload(record.ID, saluteFileID)

	// Создаем задачу на распознавание
	taskID, err := p.saluteClient.CreateTask(saluteFileID, job.AudioEncoding)
	if err != nil {
		logger.Error("Failed to create recognition task", zap.Error(err))
		p.audioRepo.UpdateResult(record.ID, "Ошибка создания задачи распознавания", "", "", models.StatusFailed)
		p.audioRepo.UpdateResult(record.ID, "Ошибка скачивания файла", "", "", models.StatusFailed)
		p.audioRepo.UpdateResult(record.ID, "Ошибка чтения файла", "", "", models.StatusFailed)
		p.audioRepo.UpdateResult(record.ID, "Ошибка загрузки в сервис распознавания", "", "", models.StatusFailed)
		p.audioRepo.UpdateResult(record.ID, "Ошибка создания задачи распознавания", "", "", models.StatusFailed)
		result.Success = false
		result.Error = "Ошибка создания задачи распознавания"
		result.Message = "❌ Не удалось создать задачу на распознавание."
		return result
	}

	p.audioRepo.UpdateAfterTaskCreated(record.ID, taskID)
	result.TaskID = taskID

	// Формируем сообщение об успешной отправке
	result.Success = true
	result.Message = fmt.Sprintf("✅ Файл отправлен на распознавание. ID встречи: `%d`\n"+
		"Когда расшифровка будет готова, я пришлю её вам.\n"+
		"Используйте `/get %d` чтобы получить текст позже.",
		record.ID, record.ID)

	logger.Info("Job processed successfully",
		zap.Int64("record_id", record.ID),
		zap.String("task_id", taskID),
	)

	return result
}

// resultSender отправляет результаты пользователям
func (p *AudioProcessor) resultSender() {
	p.logger.Info("Result sender started")

	for {
		select {
		case <-p.stopChan:
			p.logger.Info("Result sender stopped")
			return
		case result, ok := <-p.resultQueue.Results:
			if !ok {
				return
			}

			// Отправляем результат пользователю
			p.sendResultToUser(result)
		}
	}
}

// sendResultToUser отправляет результат конкретному пользователю
func (p *AudioProcessor) sendResultToUser(result queue.Result) {
	user, err := p.userRepo.GetUserByTelegramID(result.UserID)
	if err != nil || user == nil {
		p.logger.Error("Failed to get user for result",
			zap.Int64("user_id", result.UserID),
			zap.Error(err),
		)
		return
	}

	recipient := &tele.User{ID: user.TelegramID}

	// Отправляем сообщение
	if _, err := p.bot.Send(recipient, result.Message, tele.ModeMarkdown); err != nil {
		// Пробуем без Markdown
		if _, err := p.bot.Send(recipient, result.Message); err != nil {
			p.logger.Error("Failed to send result to user",
				zap.Int64("user_id", result.UserID),
				zap.Error(err),
			)
		}
	}

	p.logger.Debug("Result sent to user",
		zap.Int64("user_id", result.UserID),
		zap.String("job_id", result.JobID),
	)
}

// statusPoller опрашивает статусы задач
func (p *AudioProcessor) statusPoller() {
	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	p.logger.Info("Status poller started", zap.Duration("interval", pollingInterval))

	for {
		select {
		case <-p.stopChan:
			p.logger.Info("Status poller stopped")
			return
		case <-ticker.C:
			p.checkTaskStatuses()
		}
	}
}

// checkTaskStatuses проверяет статусы всех активных задач
func (p *AudioProcessor) checkTaskStatuses() {
	records, err := p.audioRepo.GetProcessingTasksWithTimeout(maxWaitTime)
	if err != nil {
		p.logger.Error("Failed to get processing tasks", zap.Error(err))
		return
	}

	for _, rec := range records {
		p.checkRecordStatus(rec)
	}

	// Проверяем просроченные задачи
	expiredRecords, err := p.audioRepo.GetExpiredTasks(maxWaitTime)
	if err != nil {
		p.logger.Error("Failed to get expired tasks", zap.Error(err))
		return
	}

	for _, rec := range expiredRecords {
		p.logger.Warn("Task timeout exceeded",
			zap.Int64("record_id", rec.ID),
			zap.String("task_id", rec.TaskID.String),
		)
		p.audioRepo.UpdateResult(rec.ID, "Превышено время ожидания распознавания (20 минут)", "", "", models.StatusFailed)

		// Отправляем уведомление пользователю
		if user, err := p.userRepo.GetUserByID(rec.UserID); err == nil && user != nil {
			recipient := &tele.User{ID: user.TelegramID}
			p.bot.Send(recipient, "❌ Превышено время ожидания распознавания. Попробуйте отправить файл еще раз.")
		}
	}
}

// checkRecordStatus проверяет статус одной записи
func (p *AudioProcessor) checkRecordStatus(rec models.AudioRecord) {
	logger := p.logger.With(
		zap.Int64("record_id", rec.ID),
		zap.String("task_id", rec.TaskID.String),
	)

	statusResp, err := p.saluteClient.GetTaskStatus(rec.TaskID.String)
	if err != nil {
		logger.Error("Failed to get task status", zap.Error(err))
		return
	}

	switch statusResp.Result.Status {
	case "DONE":
		if statusResp.Result.ResponseFileID == "" {
			logger.Error("Task DONE but no response_file_id")
			p.audioRepo.UpdateResult(rec.ID, "", "", "", models.StatusFailed)
			return
		}

		logger.Info("Downloading recognition result")
		resultData, err := p.saluteClient.GetRecognitionResult(statusResp.Result.ResponseFileID)
		if err != nil {
			logger.Error("Failed to download result", zap.Error(err))
			return
		}

		// ДЕБАГ: выводим первые 500 символов ответа
		logger.Info("RAW RESPONSE FROM SALUTESPEECH",
			zap.Int("size", len(resultData)),
			zap.String("preview", string(resultData[:min(500, len(resultData))])),
		)

		// Парсим ответ SaluteSpeech
		rawResponse, text, normalizedText := salutespeech.ParseRecognitionResponse(resultData)

		logger.Info("PARSED RESULT",
			zap.String("raw_response_preview", rawResponse[:min(200, len(rawResponse))]),
			zap.String("text", text),
			zap.String("normalized_text", normalizedText),
		)

		if text == "" && normalizedText == "" {
			text = "Распознавание не вернуло текст."
			normalizedText = text
		}

		// Генерируем выжимку из нормализованного текста
		var summary string
		if p.gigaClient != nil && normalizedText != "" {
			logger.Info("Generating summary with GigaChat")
			summary, err = p.gigaClient.GenerateSummary(normalizedText)
			if err != nil {
				logger.Error("Failed to generate summary", zap.Error(err))
				summary = "Не удалось сгенерировать выжимку."
			}
		} else {
			summary = "Выжимка недоступна (GigaChat не настроен или нет текста)"
		}

		// Сохраняем результат в БД
		err = p.audioRepo.UpdateResult(rec.ID, rawResponse, text, normalizedText, models.StatusCompleted)
		if err != nil {
			logger.Error("Failed to update record", zap.Error(err))
			return
		}

		// Сохраняем выжимку (если есть)
		if summary != "" && summary != "Выжимка недоступна (GigaChat не настроен или нет текста)" {
			p.audioRepo.UpdateSummary(rec.ID, summary)
		}

		// Отправляем выжимку пользователю
		if user, err := p.userRepo.GetUserByID(rec.UserID); err == nil && user != nil {
			recipient := &tele.User{ID: user.TelegramID}
			displayText := normalizedText
			if displayText == "" {
				displayText = text
			}
			message := fmt.Sprintf("🎤 *Результат распознавания встречи #%d:*\n\n%s\n\n---\n📌 Краткая выжимка: `/summary %d`\n📝 Полный текст: `/get %d`",
				rec.ID, displayText, rec.ID, rec.ID)
			p.bot.Send(recipient, message, tele.ModeMarkdown)
		}

	case "ERROR":
		errMsg := statusResp.Result.Error
		if errMsg == "" {
			errMsg = "Неизвестная ошибка"
		}
		logger.Error("Task failed", zap.String("error", errMsg))
		p.audioRepo.UpdateResult(rec.ID, "", "", "", models.StatusFailed)

		if user, err := p.userRepo.GetUserByID(rec.UserID); err == nil && user != nil {
			recipient := &tele.User{ID: user.TelegramID}
			p.bot.Send(recipient, fmt.Sprintf("❌ Ошибка распознавания: %s", errMsg))
		}

	case "NEW", "RUNNING":
		logger.Debug("Task still processing")
	}
}

// extractTextFromResult извлекает нормализованный текст из ответа API
func extractTextFromResult(resultData []byte) string {
	var response struct {
		Result []struct {
			Text           string  `json:"text"`
			NormalizedText string  `json:"normalized_text,omitempty"`
			Confidence     float64 `json:"confidence"`
		} `json:"result"`
	}

	if err := json.Unmarshal(resultData, &response); err != nil {
		// Если не удалось распарсить, возвращаем сырые данные
		return string(resultData)
	}

	if len(response.Result) == 0 {
		return ""
	}

	var texts []string
	for _, res := range response.Result {
		// Сначала пробуем normalized_text (более чистый вариант)
		if res.NormalizedText != "" {
			texts = append(texts, res.NormalizedText)
		} else if res.Text != "" {
			texts = append(texts, res.Text)
		}
	}

	// Удаляем дубликаты
	seen := make(map[string]bool)
	uniqueTexts := []string{}
	for _, s := range texts {
		if !seen[s] && s != "" {
			seen[s] = true
			uniqueTexts = append(uniqueTexts, s)
		}
	}

	if len(uniqueTexts) == 0 {
		return ""
	}

	// Объединяем все результаты
	return strings.Join(uniqueTexts, "\n")
}
