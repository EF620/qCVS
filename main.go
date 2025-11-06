package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

// Quote — структура одной цитаты
type Quote struct {
	Text          string
	Author        string
	ContextBefore string
	ContextAfter  string
}

// splitSentences — разбивает текст на предложения, игнорируя точки в скобках
func splitSentences(text string) []string {
	var sentences []string
	var sb strings.Builder
	parenLevel := 0 // Счётчик для отслеживания вложенности скобок

	for i, r := range text {
		sb.WriteRune(r)

		// Отслеживаем открытие и закрытие скобок
		switch r {
		case '(', '[', '{':
			parenLevel++
		case ')', ']', '}':
			parenLevel--
		}

		// Проверяем, является ли символ концом предложения
		if (r == '.' || r == '!' || r == '?') && parenLevel == 0 {
			// Убедимся, что это не точка внутри сокращений (например, "т.е.")
			if r == '.' && i+1 < len(text) && text[i+1] != ' ' && text[i+1] != '\n' {
				continue // Пропускаем точку, если за ней нет пробела или новой строки
			}
			s := strings.TrimSpace(sb.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			sb.Reset()
		}
	}

	// Добавляем остаток текста, если он есть
	if sb.Len() > 0 {
		s := strings.TrimSpace(sb.String())
		if s != "" {
			sentences = append(sentences, s)
		}
	}

	return sentences
}

// verifyAndExtractContext — находит цитату и возвращает контекст (2 предложения до и после)
func verifyAndExtractContext(sentences []string, quote string, author string) (Quote, bool) {
	for i, s := range sentences {
		if strings.Contains(s, quote) {
			start := i - 2
			if start < 0 {
				start = 0
			}
			end := i + 3
			if end > len(sentences) {
				end = len(sentences)
			}
			return Quote{
				Text:          quote,
				Author:        author,
				ContextBefore: strings.Join(sentences[start:i], " "),
				ContextAfter:  strings.Join(sentences[i+1:end], " "),
			}, true
		}
	}
	return Quote{}, false
}

// initCSV — создаёт CSV с заголовками
func initCSV(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Добавляем BOM для UTF-8 (опционально, для совместимости с Excel)
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}

	writer := csv.NewWriter(file)
	writer.Comma = ';' // Используем стандартный разделитель ;
	defer writer.Flush()

	// Записываем заголовки
	err = writer.Write([]string{"Цитата", "Автор", "Контекст (До)", "Контекст (После)"})
	if err != nil {
		return err
	}

	return nil
}

// appendToCSV — добавляет записи в CSV
func appendToCSV(filePath string, quotes []Quote) error {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Comma = ';' // Используем стандартный разделитель ;
	defer writer.Flush()

	for _, q := range quotes {
		// Экранируем символы ; в данных, заменяя их на запятые
		text := strings.ReplaceAll(q.Text, ";", ",")
		author := strings.ReplaceAll(q.Author, ";", ",")
		contextBefore := strings.ReplaceAll(q.ContextBefore, ";", ",")
		contextAfter := strings.ReplaceAll(q.ContextAfter, ";", ",")

		// Также заменяем переносы строк, чтобы не ломать CSV
		text = strings.ReplaceAll(text, "\n", " ")
		author = strings.ReplaceAll(author, "\n", " ")
		contextBefore = strings.ReplaceAll(contextBefore, "\n", " ")
		contextAfter = strings.ReplaceAll(contextAfter, "\n", " ")

		// Записываем данные
		err := writer.Write([]string{text, author, contextBefore, contextAfter})
		if err != nil {
			return err
		}
	}

	// Принудительно сбрасываем буфер
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	return nil
}

// extractQuotesFromAI — вызывает Gemini и получает список цитат
func extractQuotesFromAI(ctx context.Context, client *genai.Client, text string) ([]string, error) {
	model := "gemini-2.5-flash"
	prompt := `Извлеки из текста 3-10 ярких, выразительных цитат. 
Ответ верни строго в формате JSON массива строк. Пример: ["цитата1", "цитата2"]. 
Не добавляй лишних символов, обратных кавычек или пояснений.`

	resp, err := client.Models.GenerateContent(ctx, model, genai.Text(prompt+"\n\nТекст:\n"+text), nil)
	if err != nil {
		return nil, err
	}

	response := strings.TrimSpace(resp.Text())
	// очищаем возможные ```json и прочее
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var quotes []string
	if err := json.Unmarshal([]byte(response), &quotes); err != nil {
		log.Printf("⚠️ Ответ не JSON, пропускаю блок. Ответ: %s", response)
		return nil, nil
	}
	return quotes, nil
}

func processFile(filePath, author string, client *genai.Client) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	outputFile := strings.TrimSuffix(filePath, ".txt") + ".csv"
	if err := initCSV(outputFile); err != nil {
		return err
	}

	var block strings.Builder
	scanner := bufio.NewScanner(file)
	const maxBlockSize = 3000
	allQuotes := []Quote{}
	allSentences := []string{}

	for scanner.Scan() {
		line := scanner.Text()
		allSentences = append(allSentences, splitSentences(line)...)
		block.WriteString(line + "\n")
		if block.Len() > maxBlockSize {
			if err := processBlock(block.String(), allSentences, author, client, outputFile, &allQuotes); err != nil {
				log.Println("Ошибка:", err)
			}
			block.Reset()
		}
	}

	if block.Len() > 0 {
		processBlock(block.String(), allSentences, author, client, outputFile, &allQuotes)
	}

	log.Printf("✅ Всего сохранено цитат: %d", len(allQuotes))
	return nil
}

func processBlock(text string, sentences []string, author string, client *genai.Client, csvPath string, allQuotes *[]Quote) error {
	ctx := context.Background()
	log.Printf("⚙️ Обработка блока (%d символов)...", len(text))

	aiQuotes, err := extractQuotesFromAI(ctx, client, text)
	if err != nil {
		return fmt.Errorf("ошибка AI: %v", err)
	}
	if len(aiQuotes) == 0 {
		return nil
	}

	validQuotes := []Quote{}
	for _, q := range aiQuotes {
		quote, ok := verifyAndExtractContext(sentences, q, author)
		if ok {
			validQuotes = append(validQuotes, quote)
		}
	}

	if len(validQuotes) > 0 {
		appendToCSV(csvPath, validQuotes)
		*allQuotes = append(*allQuotes, validQuotes...)
		log.Printf("✅ Сохранено %d цитат", len(validQuotes))
	}

	time.Sleep(1 * time.Second)
	return nil
}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Использование: go run main.go <путь_к_файлу.txt> <Автор>")
	}

	filePath := os.Args[1]
	author := os.Args[2]

	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Fatal("Не найден GOOGLE_API_KEY в окружении.")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		log.Fatalf("Ошибка создания клиента Gemini: %v", err)
	}

	if err := processFile(filePath, author, client); err != nil {
		log.Fatalf("Ошибка обработки файла: %v", err)
	}
}
