package bot

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/IvanDolgov/go-final-project/internal/gigachat"
	"github.com/IvanDolgov/go-final-project/internal/models"
	"github.com/IvanDolgov/go-final-project/internal/queue"
	"github.com/IvanDolgov/go-final-project/internal/salutespeech"
	"github.com/IvanDolgov/go-final-project/internal/storage"
	"github.com/IvanDolgov/go-final-project/internal/worker"
	"github.com/google/uuid"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

type BotHandlers struct {
	userRepo       *storage.UserRepository
	audioRepo      *storage.AudioRepository
	saluteClient   *salutespeech.Client
	gigaClient     *gigachat.Client
	logger         *zap.Logger
	audioProcessor *worker.AudioProcessor
}

func NewBotHandlers(
	userRepo *storage.UserRepository,
	audioRepo *storage.AudioRepository,
	saluteClient *salutespeech.Client,
	gigaClient *gigachat.Client,
	logger *zap.Logger,
	audioProcessor *worker.AudioProcessor,
) *BotHandlers {
	return &BotHandlers{
		userRepo:       userRepo,
		audioRepo:      audioRepo,
		saluteClient:   saluteClient,
		gigaClient:     gigaClient,
		logger:         logger.With(zap.String("handlers", "bot")),
		audioProcessor: audioProcessor,
	}
}

// Register регистрирует все обработчики команд
func (h *BotHandlers) Register(b *tele.Bot) {
	b.Handle("/start", h.handleStart)
	b.Handle("/list", h.handleList)
	b.Handle("/get", h.handleGet)
	b.Handle("/chat", h.handleChat)
	b.Handle("/summary", h.handleSqueeze)
	b.Handle(tele.OnAudio, h.handleAudio)
	b.Handle(tele.OnVoice, h.handleVoice)
}

// handleStart обрабатывает команду /start
func (h *BotHandlers) handleStart(c tele.Context) error {
	user := c.Sender()

	logger := h.logger.With(
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
		zap.String("first_name", user.FirstName),
	)

	logger.Info("Received /start command")

	// Сохраняем пользователя в БД
	if err := h.userRepo.SaveUser(user.ID); err != nil {
		logger.Error("Error saving user", zap.Error(err))
		return c.Send("Произошла ошибка при сохранении данных. Попробуйте позже.")
	}

	message := "🎤 *Добро пожаловать в бот для распознавания встреч!*\n\n" +
		"Я помогу вам расшифровывать аудиозаписи встреч.\n\n" +
		"*Доступные команды:*\n" +
		"📋 `/list` - список ваших сохраненных встреч\n" +
		"🔍 `/get [ID]` - получить текст встречи по ID\n\n" +
		"*Как пользоваться:*\n" +
		"1. Отправьте мне голосовое сообщение или аудиофайл\n" +
		"2. Дождитесь окончания обработки\n" +
		"3. Используйте `/list` чтобы увидеть все встречи\n" +
		"4. Используйте `/get 123` чтобы прочитать текст конкретной встречи"

	return c.Send(message, tele.ModeMarkdown)
}

// handleList обрабатывает команду /list - показывает все встречи пользователя
func (h *BotHandlers) handleList(c tele.Context) error {
	user := c.Sender()

	logger := h.logger.With(
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
	)

	logger.Info("Received /list command")

	// Получаем внутренний ID пользователя
	userDBID, err := h.userRepo.GetOrCreateUser(user.ID)
	if err != nil {
		logger.Error("Failed to get user", zap.Error(err))
		return c.Send("❌ Ошибка при получении данных пользователя.")
	}

	// Получаем список встреч пользователя
	records, err := h.audioRepo.GetUserRecords(userDBID)
	if err != nil {
		logger.Error("Failed to get user records", zap.Error(err))
		return c.Send("❌ Ошибка при получении списка встреч.")
	}

	if len(records) == 0 {
		return c.Send("📭 У вас пока нет сохраненных встреч. Отправьте голосовое сообщение или аудиофайл для распознавания.")
	}

	// Формируем сообщение со списком встреч
	var message strings.Builder
	message.WriteString("📋 *Ваши сохраненные встречи:*\n\n")

	for i, record := range records {
		// Форматируем дату
		dateStr := record.CreatedAt.Format("02.01.2006 15:04")

		// Добавляем статус
		statusEmoji := "✅"
		if record.Status != models.StatusCompleted {
			statusEmoji = "⏳"
		}

		// Добавляем запись в список
		message.WriteString(fmt.Sprintf("%d. `%d` %s %s\n",
			i+1, record.ID, statusEmoji, dateStr))

		// Добавляем превью текста, если есть
		if record.RecognitionText.Valid && len(record.RecognitionText.String) > 0 {
			preview := record.RecognitionText.String
			if len(preview) > 50 {
				preview = preview[:50] + "..."
			}
			// Экранируем спецсимволы для Markdown
			preview = escapeMarkdown(preview)
			message.WriteString(fmt.Sprintf("   📝 %s\n", preview))
		}
	}

	message.WriteString("\n📌 Для просмотра полного текста используйте:\n`/get [ID встречи]`")

	// Отправляем с Markdown
	return c.Send(message.String(), tele.ModeMarkdown)
}

// handleGet обрабатывает команду /get [ID] - показывает текст конкретной встречи
func (h *BotHandlers) handleGet(c tele.Context) error {
	user := c.Sender()
	args := c.Args()

	logger := h.logger.With(
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
		zap.Any("args", args),
	)

	logger.Info("Received /get command")

	// Проверяем наличие аргумента
	if len(args) == 0 {
		return c.Send("❌ Укажите ID встречи. Пример: /get 123")
	}

	// Парсим ID встречи
	recordID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный формат ID. ID должен быть числом.")
	}

	// Получаем внутренний ID пользователя
	userDBID, err := h.userRepo.GetOrCreateUser(user.ID)
	if err != nil {
		logger.Error("Failed to get user", zap.Error(err))
		return c.Send("❌ Ошибка при получении данных пользователя.")
	}

	// Получаем запись по ID
	record, err := h.audioRepo.GetRecordByID(recordID)
	if err != nil {
		logger.Error("Failed to get record", zap.Error(err), zap.Int64("record_id", recordID))
		return c.Send("❌ Ошибка при получении записи.")
	}

	// Проверяем, существует ли запись
	if record == nil {
		return c.Send("❌ Запись с таким ID не найдена.")
	}

	// Проверяем, принадлежит ли запись пользователю
	if record.UserID != userDBID {
		logger.Warn("User attempted to access another user's record",
			zap.Int64("record_owner_id", record.UserID),
			zap.Int64("requesting_user_id", userDBID))
		return c.Send("❌ У вас нет доступа к этой записи.")
	}

	// Проверяем статус записи
	if record.Status != models.StatusCompleted {
		statusMsg := "⏳ Запись еще обрабатывается"
		switch record.Status {
		case models.StatusPending:
			statusMsg = "⏳ Запись ожидает обработки"
		case models.StatusProcessing:
			statusMsg = "⏳ Запись обрабатывается (это может занять до 20 минут)"
		case models.StatusFailed:
			statusMsg = "❌ Обработка записи завершилась ошибкой"
		}
		return c.Send(statusMsg)
	}

	// Проверяем наличие текста
	if !record.RecognitionText.Valid || record.RecognitionText.String == "" {
		return c.Send("❌ Текст встречи отсутствует.")
	}

	// Формируем сообщение с текстом встречи
	dateStr := record.CreatedAt.Format("02.01.2006 15:04")

	// Экранируем текст для Markdown
	escapedText := escapeMarkdown(record.RecognitionText.String)

	// Формируем сообщение
	message := fmt.Sprintf("📝 *Полный текст встречи от %s (ID: %d)*\n\n%s",
		dateStr, record.ID, escapedText)

	// Отправляем результат с Markdown
	const maxMessageLength = 4096

	if len(message) <= maxMessageLength {
		return c.Send(message, tele.ModeMarkdown)
	}

	// Если текст слишком длинный, разбиваем на части
	parts := splitMessage(message, maxMessageLength)
	for i, part := range parts {
		if i == 0 {
			c.Send(part, tele.ModeMarkdown)
		} else {
			c.Send(part)
		}
	}

	return nil
}

// escapeMarkdown экранирует специальные символы для MarkdownV2
func escapeMarkdown(text string) string {
	// Специальные символы в MarkdownV2: _ * [ ] ( ) ~ ` > # + - = | { } . !
	specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}

	result := text
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// splitMessage разбивает длинное сообщение на части
func splitMessage(message string, maxLen int) []string {
	var parts []string
	for len(message) > 0 {
		if len(message) <= maxLen {
			parts = append(parts, message)
			break
		}
		// Ищем последний пробел перед maxLen
		cutIndex := strings.LastIndex(message[:maxLen], "\n")
		if cutIndex == -1 {
			cutIndex = strings.LastIndex(message[:maxLen], " ")
		}
		if cutIndex == -1 {
			cutIndex = maxLen
		}
		parts = append(parts, message[:cutIndex])
		message = message[cutIndex:]
	}
	return parts
}

// handleVoice обрабатывает голосовые сообщения
func (h *BotHandlers) handleVoice(c tele.Context) error {
	user := c.Sender()
	voice := c.Message().Voice

	logger := h.logger.With(
		zap.Int64("user_id", user.ID),
		zap.String("file_id", voice.FileID),
	)

	logger.Info("Received voice message")

	// Получаем внутренний ID пользователя
	userDBID, err := h.userRepo.GetOrCreateUser(user.ID)
	if err != nil {
		logger.Error("Failed to get user", zap.Error(err))
		return c.Send("❌ Ошибка при идентификации пользователя.")
	}

	// Создаем задачу
	job := queue.Job{
		ID:            uuid.New().String(),
		UserID:        user.ID,
		UserDBID:      userDBID,
		FileID:        voice.FileID,
		FileName:      "voice.ogg",
		AudioEncoding: "OPUS",
		Chat:          c,
		CreatedAt:     time.Now(),
	}

	// Отправляем в очередь
	h.audioProcessor.SubmitJob(job)

	// Отвечаем пользователю, что задача принята
	return c.Send("🎤 Голосовое сообщение принято в обработку. Я сообщу, когда расшифровка будет готова.")
}

// handleAudio обрабатывает аудиофайлы
func (h *BotHandlers) handleAudio(c tele.Context) error {
	user := c.Sender()
	audio := c.Message().Audio

	logger := h.logger.With(
		zap.Int64("user_id", user.ID),
		zap.String("file_id", audio.FileID),
		zap.String("file_name", audio.FileName),
	)

	logger.Info("Received audio file")

	// Получаем внутренний ID пользователя
	userDBID, err := h.userRepo.GetOrCreateUser(user.ID)
	if err != nil {
		logger.Error("Failed to get user", zap.Error(err))
		return c.Send("❌ Ошибка при идентификации пользователя.")
	}

	// Скачиваем файл для определения формата
	file, err := c.Bot().File(&tele.File{FileID: audio.FileID})
	if err != nil {
		return c.Send("❌ Не удалось скачать файл для определения формата.")
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		return c.Send("❌ Ошибка при чтении файла.")
	}

	// Определяем кодек
	audioEncoding := h.detectAudioEncoding(audio.MIME, audio.FileName, fileData)

	logger.Info("Detected audio encoding", zap.String("encoding", audioEncoding))

	// Создаем задачу
	job := queue.Job{
		ID:            uuid.New().String(),
		UserID:        user.ID,
		UserDBID:      userDBID,
		FileID:        audio.FileID,
		FileName:      audio.FileName,
		AudioEncoding: audioEncoding,
		Chat:          c,
		CreatedAt:     time.Now(),
	}

	// Отправляем в очередь
	h.audioProcessor.SubmitJob(job)

	// Отвечаем пользователю, что задача принята
	return c.Send("🎵 Аудиофайл принят в обработку. Я сообщу, когда расшифровка будет готова.")
}

func (h *BotHandlers) detectAudioEncoding(mimeType, fileName string, fileData []byte) string {
	// Сначала пробуем по MIME
	mimeLower := strings.ToLower(mimeType)

	if strings.Contains(mimeLower, "ogg") || strings.Contains(mimeLower, "opus") {
		return "OPUS"
	}
	if strings.Contains(mimeLower, "mpeg") || strings.Contains(mimeLower, "mp3") {
		return "MP3"
	}
	if strings.Contains(mimeLower, "wav") {
		return "PCM_S16LE"
	}
	if strings.Contains(mimeLower, "flac") {
		return "FLAC"
	}

	// По расширению
	fileNameLower := strings.ToLower(fileName)
	if strings.HasSuffix(fileNameLower, ".mp3") {
		return "MP3"
	}
	if strings.HasSuffix(fileNameLower, ".ogg") || strings.HasSuffix(fileNameLower, ".opus") {
		return "OPUS"
	}
	if strings.HasSuffix(fileNameLower, ".wav") {
		return "PCM_S16LE"
	}
	if strings.HasSuffix(fileNameLower, ".flac") {
		return "FLAC"
	}

	// По магическим байтам
	if len(fileData) > 4 {
		// OGG
		if fileData[0] == 0x4F && fileData[1] == 0x67 && fileData[2] == 0x67 && fileData[3] == 0x53 {
			return "OPUS"
		}
		// MP3 ID3
		if fileData[0] == 0x49 && fileData[1] == 0x44 && fileData[2] == 0x33 {
			return "MP3"
		}
		// FLAC
		if fileData[0] == 0x66 && fileData[1] == 0x4C && fileData[2] == 0x61 && fileData[3] == 0x43 {
			return "FLAC"
		}
		// WAV
		if fileData[0] == 0x52 && fileData[1] == 0x49 && fileData[2] == 0x46 && fileData[3] == 0x46 {
			return "PCM_S16LE"
		}
	}

	// По умолчанию
	return "MP3"
}

// handleChat обрабатывает команду /chat <вопрос>
func (h *BotHandlers) handleChat(c tele.Context) error {
	user := c.Sender()
	args := c.Args()

	logger := h.logger.With(
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
	)

	logger.Info("Received /chat command", zap.Any("args", args))

	// Проверяем наличие текста запроса
	if len(args) == 0 {
		return c.Send("❌ Укажите вопрос для GigaChat. Пример: `/chat Что такое искусственный интеллект?`", tele.ModeMarkdown)
	}

	// Собираем текст вопроса (все аргументы)
	query := strings.Join(args, " ")

	// Отправляем сообщение о начале обработки
	thinkingMsg, err := c.Bot().Send(c.Sender(), "🤔 Думаю...")
	if err != nil {
		logger.Error("Failed to send thinking message", zap.Error(err))
	}

	// Проверяем наличие клиента
	if h.gigaClient == nil {
		logger.Error("GigaChat client is nil")
		if thinkingMsg != nil {
			c.Bot().Delete(thinkingMsg)
		}
		return c.Send("❌ Сервис GigaChat временно недоступен. Попробуйте позже.")
	}

	// Отправляем запрос к GigaChat
	answer, err := h.gigaClient.Chat(query)
	if err != nil {
		logger.Error("Failed to get response from GigaChat", zap.Error(err))
		if thinkingMsg != nil {
			c.Bot().Delete(thinkingMsg)
		}
		return c.Send("❌ Ошибка при обращении к GigaChat. Попробуйте позже.")
	}

	// Удаляем сообщение "Думаю..."
	if thinkingMsg != nil {
		c.Bot().Delete(thinkingMsg)
	}

	// Формируем ответ
	message := fmt.Sprintf("🤖 *GigaChat отвечает:*\n\n%s", answer)

	// Разбиваем длинное сообщение
	const maxLen = 4096
	if len(message) <= maxLen {
		return c.Send(message, tele.ModeMarkdown)
	}

	// Отправляем по частям
	parts := splitMessage(message, maxLen)
	for i, part := range parts {
		if i == 0 {
			c.Send(part, tele.ModeMarkdown)
		} else {
			c.Send(part)
		}
	}

	return nil
}

// handleSqueeze обрабатывает команду /summary <id> - показывает выжимку встречи
func (h *BotHandlers) handleSqueeze(c tele.Context) error {
	user := c.Sender()
	args := c.Args()

	logger := h.logger.With(
		zap.Int64("user_id", user.ID),
		zap.String("username", user.Username),
	)

	logger.Info("Received /summary command", zap.Any("args", args))

	// Проверяем наличие аргумента
	if len(args) == 0 {
		return c.Send("❌ Укажите ID встречи. Пример: `/summary 123`", tele.ModeMarkdown)
	}

	// Парсим ID встречи
	recordID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный формат ID. ID должен быть числом.")
	}

	// Получаем внутренний ID пользователя
	userDBID, err := h.userRepo.GetOrCreateUser(user.ID)
	if err != nil {
		logger.Error("Failed to get user", zap.Error(err))
		return c.Send("❌ Ошибка при получении данных пользователя.")
	}

	// Получаем запись по ID
	record, err := h.audioRepo.GetRecordByID(recordID)
	if err != nil {
		logger.Error("Failed to get record", zap.Error(err), zap.Int64("record_id", recordID))
		return c.Send("❌ Ошибка при получении записи.")
	}

	// Проверяем, существует ли запись
	if record == nil {
		return c.Send("❌ Запись с таким ID не найдена.")
	}

	// Проверяем, принадлежит ли запись пользователю
	if record.UserID != userDBID {
		logger.Warn("User attempted to access another user's record",
			zap.Int64("record_owner_id", record.UserID),
			zap.Int64("requesting_user_id", userDBID))
		return c.Send("❌ У вас нет доступа к этой записи.")
	}

	// Проверяем статус записи
	if record.Status != models.StatusCompleted {
		statusMsg := "⏳ Запись еще обрабатывается"
		switch record.Status {
		case models.StatusPending:
			statusMsg = "⏳ Запись ожидает обработки"
		case models.StatusProcessing:
			statusMsg = "⏳ Запись обрабатывается (это может занять до 20 минут)"
		case models.StatusFailed:
			statusMsg = "❌ Обработка записи завершилась ошибкой"
		}
		return c.Send(statusMsg)
	}

	// Если выжимки нет, генерируем её
	if !record.Summary.Valid || record.Summary.String == "" {
		if !record.RecognitionText.Valid || record.RecognitionText.String == "" {
			return c.Send("❌ Текст встречи отсутствует. Не могу сгенерировать выжимку.")
		}

		if h.gigaClient == nil {
			return c.Send("❌ Сервис GigaChat недоступен. Не могу сгенерировать выжимку.")
		}

		logger.Info("No summary found, generating new one")
		c.Send("🤔 Генерирую краткую выжимку встречи...")

		summary, err := h.gigaClient.GenerateSummary(record.RecognitionText.String)
		if err != nil {
			logger.Error("Failed to generate summary", zap.Error(err))
			return c.Send("❌ Ошибка при генерации выжимки. Попробуйте позже.")
		}

		// Сохраняем выжимку в БД
		if err := h.audioRepo.UpdateSummary(record.ID, summary); err != nil {
			logger.Error("Failed to save summary", zap.Error(err))
		}

		// Формируем ответ
		dateStr := record.CreatedAt.Format("02.01.2006 15:04")
		message := fmt.Sprintf("📝 *Краткая выжимка встречи от %s (ID: %d)*\n\n%s\n\n---\n📌 Полный текст: `/get %d`",
			dateStr, record.ID, summary, record.ID)

		return c.Send(message, tele.ModeMarkdown)
	}

	// Если выжимка есть, отправляем её
	dateStr := record.CreatedAt.Format("02.01.2006 15:04")
	escapedSummary := escapeMarkdown(record.Summary.String)
	message := fmt.Sprintf("📝 *Краткая выжимка встречи от %s (ID: %d)*\n\n%s\n\n---\n📌 Полный текст: `/get %d`",
		dateStr, record.ID, escapedSummary, record.ID)

	return c.Send(message, tele.ModeMarkdown)
}
