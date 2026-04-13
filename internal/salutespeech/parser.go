package salutespeech

import (
	"encoding/json"
	"strings"
)

// ParseRecognitionResponse парсит ответ от SaluteSpeech и извлекает текст
func ParseRecognitionResponse(responseData []byte) (rawResponse string, text string, normalizedText string) {
	rawResponse = string(responseData)

	// Пробуем распарсить как массив
	var arrayResponse []struct {
		Results []struct {
			Text           string  `json:"text"`
			NormalizedText string  `json:"normalized_text"`
			Confidence     float64 `json:"confidence"`
		} `json:"results"`
	}

	if err := json.Unmarshal(responseData, &arrayResponse); err == nil && len(arrayResponse) > 0 {
		var texts []string
		var normalizedTexts []string

		for _, item := range arrayResponse {
			for _, res := range item.Results {
				if res.Text != "" {
					texts = append(texts, res.Text)
				}
				if res.NormalizedText != "" {
					normalizedTexts = append(normalizedTexts, res.NormalizedText)
				}
			}
		}

		text = strings.Join(texts, "\n")
		normalizedText = strings.Join(normalizedTexts, "\n")
		return
	}

	// Пробуем как объект с полем result
	var response struct {
		Result []struct {
			Text           string  `json:"text"`
			NormalizedText string  `json:"normalized_text"`
			Confidence     float64 `json:"confidence"`
		} `json:"result"`
	}

	if err := json.Unmarshal(responseData, &response); err == nil && len(response.Result) > 0 {
		var texts []string
		var normalizedTexts []string

		for _, res := range response.Result {
			if res.Text != "" {
				texts = append(texts, res.Text)
			}
			if res.NormalizedText != "" {
				normalizedTexts = append(normalizedTexts, res.NormalizedText)
			}
		}

		text = strings.Join(texts, "\n")
		normalizedText = strings.Join(normalizedTexts, "\n")
		return
	}

	// Пробуем как объект с полем results
	var altResponse struct {
		Results []struct {
			Text           string  `json:"text"`
			NormalizedText string  `json:"normalized_text"`
			Confidence     float64 `json:"confidence"`
		} `json:"results"`
	}

	if err := json.Unmarshal(responseData, &altResponse); err == nil && len(altResponse.Results) > 0 {
		var texts []string
		var normalizedTexts []string

		for _, res := range altResponse.Results {
			if res.Text != "" {
				texts = append(texts, res.Text)
			}
			if res.NormalizedText != "" {
				normalizedTexts = append(normalizedTexts, res.NormalizedText)
			}
		}

		text = strings.Join(texts, "\n")
		normalizedText = strings.Join(normalizedTexts, "\n")
		return
	}

	// Если ничего не подошло, возвращаем как есть
	text = rawResponse
	normalizedText = rawResponse
	return
}
