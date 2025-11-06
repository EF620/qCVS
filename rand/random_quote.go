package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Quote — структура для хранения цитаты из CSV
type Quote struct {
	Text          string
	Author        string
	ContextBefore string
	ContextAfter  string
}

// findCSVFiles — рекурсивно ищет все CSV-файлы в папке и подпапках
func findCSVFiles(rootDir string) ([]string, error) {
	var csvFiles []string
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".csv") {
			csvFiles = append(csvFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ошибка при поиске CSV-файлов: %v", err)
	}
	return csvFiles, nil
}

// readQuotesFromCSV — читает все цитаты из одного CSV-файла
func readQuotesFromCSV(filePath string) ([]Quote, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка при открытии файла %s: %v", filePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';' // Устанавливаем разделитель ;
	reader.TrimLeadingSpace = true

	// Читаем все записи
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("ошибка при чтении CSV %s: %v", filePath, err)
	}

	var quotes []Quote
	// Пропускаем заголовок (первая строка)
	for i, record := range records[1:] {
		if len(record) < 4 {
			log.Printf("Предупреждение: строка %d в файле %s содержит недостаточно полей", i+2, filePath)
			continue
		}
		quotes = append(quotes, Quote{
			Text:          record[0],
			Author:        record[1],
			ContextBefore: record[2],
			ContextAfter:  record[3],
		})
	}

	return quotes, nil
}

// getRandomQuote — возвращает случайную цитату из всех CSV-файлов
func getRandomQuote(rootDir string) (Quote, error) {
	// Находим все CSV-файлы
	csvFiles, err := findCSVFiles(rootDir)
	if err != nil {
		return Quote{}, err
	}
	if len(csvFiles) == 0 {
		return Quote{}, fmt.Errorf("в папке %s не найдено CSV-файлов", rootDir)
	}

	// Собираем все цитаты из всех файлов
	var allQuotes []Quote
	for _, filePath := range csvFiles {
		quotes, err := readQuotesFromCSV(filePath)
		if err != nil {
			log.Printf("Ошибка при чтении файла %s: %v", filePath, err)
			continue
		}
		allQuotes = append(allQuotes, quotes...)
	}

	if len(allQuotes) == 0 {
		return Quote{}, fmt.Errorf("не найдено цитат в CSV-файлах")
	}

	// Выбираем случайную цитату
	rand.Seed(time.Now().UnixNano())
	randomIndex := rand.Intn(len(allQuotes))
	return allQuotes[randomIndex], nil
}

func main() {
	// Флаги командной строки
	jsonOutput := flag.Bool("json", false, "Выводить цитату в формате JSON")
	flag.Parse()

	if flag.NArg() < 1 {
		log.Fatal("Использование: go run random_quote.go [--json] <путь_к_папке>")
	}
	rootDir := flag.Arg(0)

	quote, err := getRandomQuote(rootDir)
	if err != nil {
		log.Fatalf("Ошибка: %v", err)
	}

	if *jsonOutput {
		// Выводим в формате JSON
		data, err := json.MarshalIndent(quote, "", "  ")
		if err != nil {
			log.Fatalf("Ошибка при формировании JSON: %v", err)
		}
		fmt.Println(string(data))
	} else {
		// Вывод в стандартном текстовом формате
		fmt.Printf("Цитата: %s\n", quote.Text)
		fmt.Printf("Автор: %s\n", quote.Author)
		fmt.Printf("Контекст (До): %s\n", quote.ContextBefore)
		fmt.Printf("Контекст (После): %s\n", quote.ContextAfter)
	}
}
