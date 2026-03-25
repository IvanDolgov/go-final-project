package audio

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ConvertToPCM конвертирует любое аудио в PCM S16LE 16000Hz моно
func ConvertToPCM(inputData []byte, inputFileName string) ([]byte, error) {
	// Создаем временные файлы
	tmpInput, err := os.CreateTemp("", "input-*"+getExtension(inputFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer os.Remove(tmpInput.Name())
	defer tmpInput.Close()

	tmpOutput, err := os.CreateTemp("", "output-*.raw")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp output file: %w", err)
	}
	defer os.Remove(tmpOutput.Name())
	defer tmpOutput.Close()

	// Записываем входные данные
	if _, err := tmpInput.Write(inputData); err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}
	tmpInput.Close()

	// Логируем размер файла
	fmt.Printf("Converting file: %s, size: %d bytes\n", tmpInput.Name(), len(inputData))

	// Запускаем ffmpeg для конвертации
	cmd := exec.Command("ffmpeg",
		"-i", tmpInput.Name(),
		"-ar", "16000", // частота дискретизации 16kHz (оптимально для речи)
		"-ac", "1", // моно
		"-f", "s16le", // PCM signed 16-bit little-endian
		"-acodec", "pcm_s16le",
		"-y", // перезаписывать выходной файл
		tmpOutput.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	fmt.Println("Running ffmpeg command:", cmd.String())

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg conversion failed: %w\nstderr: %s", err, stderr.String())
	}

	// Читаем результат
	outputData, err := os.ReadFile(tmpOutput.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	fmt.Printf("Conversion successful: %d bytes -> %d bytes\n", len(inputData), len(outputData))

	return outputData, nil
}

// getExtension возвращает расширение файла
func getExtension(filename string) string {
	if filename == "" {
		return ".bin"
	}
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		return "." + parts[len(parts)-1]
	}
	return ".bin"
}

// DetectFormat определяет формат файла по содержимому
func DetectFormat(data []byte) string {
	if len(data) < 4 {
		return "unknown"
	}

	// OGG заголовок (Opus в OGG)
	if bytes.HasPrefix(data, []byte("OggS")) {
		return "ogg"
	}
	// MP3 ID3 заголовок
	if bytes.HasPrefix(data, []byte("ID3")) {
		return "mp3"
	}
	// MP3 без ID3 (первые байты - FF FB или FF F3)
	if len(data) > 2 && data[0] == 0xFF && (data[1]&0xF0) == 0xF0 {
		return "mp3"
	}
	// FLAC заголовок
	if bytes.HasPrefix(data, []byte("fLaC")) {
		return "flac"
	}
	// WAV заголовок
	if bytes.HasPrefix(data, []byte("RIFF")) && len(data) > 8 && string(data[8:12]) == "WAVE" {
		return "wav"
	}
	// Opus (чистый Opus без контейнера - редкий случай)
	if len(data) > 8 && string(data[0:8]) == "OpusHead" {
		return "opus"
	}

	return "unknown"
}
