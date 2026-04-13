package queue

import (
	"time"

	tele "gopkg.in/telebot.v3"
)

// Job представляет задачу на обработку аудио
type Job struct {
	ID            string       // Уникальный ID задачи
	UserID        int64        // Telegram ID пользователя
	UserDBID      int64        // Внутренний ID пользователя в БД
	FileID        string       // Telegram file ID
	FileName      string       // Имя файла
	AudioEncoding string       // Кодек аудио
	Chat          tele.Context // Контекст чата для отправки ответа
	CreatedAt     time.Time    // Время создания задачи
}

// Result представляет результат обработки задачи
type Result struct {
	JobID    string // ID задачи
	UserID   int64  // Telegram ID пользователя
	Success  bool   // Успешна ли обработка
	RecordID int64  // ID записи в БД (если успешно)
	TaskID   string // ID задачи в SaluteSpeech (если успешно)
	Error    string // Сообщение об ошибке (если есть)
	Message  string // Сообщение для отправки пользователю
}

// JobQueue - канал для задач
type JobQueue struct {
	Jobs chan Job
}

// ResultQueue - канал для результатов
type ResultQueue struct {
	Results chan Result
}

// NewJobQueue создает новую очередь задач
func NewJobQueue(bufferSize int) *JobQueue {
	return &JobQueue{
		Jobs: make(chan Job, bufferSize),
	}
}

// NewResultQueue создает новую очередь результатов
func NewResultQueue(bufferSize int) *ResultQueue {
	return &ResultQueue{
		Results: make(chan Result, bufferSize),
	}
}

// Push добавляет задачу в очередь
func (q *JobQueue) Push(job Job) {
	q.Jobs <- job
}

// Pop забирает задачу из очереди (блокирующий)
func (q *JobQueue) Pop() Job {
	return <-q.Jobs
}

// TryPop пытается забрать задачу (неблокирующий)
func (q *JobQueue) TryPop() (Job, bool) {
	select {
	case job := <-q.Jobs:
		return job, true
	default:
		return Job{}, false
	}
}

// PushResult добавляет результат в очередь
func (q *ResultQueue) PushResult(result Result) {
	q.Results <- result
}

// PopResult забирает результат из очереди (блокирующий)
func (q *ResultQueue) PopResult() Result {
	return <-q.Results
}

// Close закрывает очереди
func (q *JobQueue) Close() {
	close(q.Jobs)
}

func (q *ResultQueue) Close() {
	close(q.Results)
}
