package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	// Конфигурация
	rootDir := "."
	currentTime := time.Now()
	outputFile := currentTime.Format("20060102_150405") + ".txt"                                           // откуда собирать
	extensions := []string{".go", ".mod", ".md", ".tmpl", ".html", ".xml"}                                 // нужные расширения
	ignoreDirs := []string{".git", ".github", "vendor", "node_modules", "tmp", "dist", ".idea", ".vscode"} // игнорируемые папки

	fmt.Printf("Сборка проекта из %s в %s\n", rootDir, outputFile)
	fmt.Printf("Расширения: %v\n", extensions)
	fmt.Printf("Игнорируем папки: %v\n", ignoreDirs)

	// Создаем или открываем выходной файл
	out, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Ошибка создания файла: %v\n", err)
		return
	}
	defer out.Close()

	// Записываем заголовок
	writeHeader(out, rootDir)

	// Собираем все файлы
	var files []string
	err = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Проверяем, нужно ли игнорировать директорию
		if d.IsDir() {
			for _, ignore := range ignoreDirs {
				if d.Name() == ignore {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Проверяем расширение файла
		ext := strings.ToLower(filepath.Ext(path))
		for _, validExt := range extensions {
			if ext == validExt {
				files = append(files, path)
				break
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Ошибка при обходе файлов: %v\n", err)
		return
	}

	// Сортируем файлы для удобства
	// (можно добавить сортировку, если нужно)

	// Обрабатываем каждый файл
	for _, file := range files {
		if err := processFile(out, file); err != nil {
			fmt.Printf("Ошибка обработки %s: %v\n", file, err)
		}
	}

	// Записываем статистику
	stats := fmt.Sprintf("\n\n===========================================\n"+
		"Статистика:\n"+
		"Всего файлов: %d\n"+
		"Дата сборки: %s\n"+
		"===========================================\n",
		len(files), time.Now().Format("2006-01-02 15:04:05"))

	out.WriteString(stats)

	fmt.Printf("\nГотово! Обработано файлов: %d\n", len(files))
	fmt.Printf("Результат сохранен в: %s\n", outputFile)
}

// writeHeader записывает общую информацию о проекте
func writeHeader(out *os.File, rootDir string) {
	header := fmt.Sprintf(`===========================================
СБОРКА ПРОЕКТА GO
===========================================
Дата: %s
Директория: %s

СТРУКТУРА ПРОЕКТА:
===========================================

`, time.Now().Format("2006-01-02 15:04:05"), rootDir)

	out.WriteString(header)

	// Добавляем дерево проекта (упрощенное)
	tree := getProjectTree(rootDir)
	out.WriteString(tree)
	out.WriteString("\n\n")
}

// getProjectTree создает упрощенное дерево проекта
func getProjectTree(rootDir string) string {
	var buf bytes.Buffer
	buf.WriteString("Дерево файлов:\n")

	ignoreDirs := map[string]bool{
		".git": true, "vendor": true, "node_modules": true,
		"tmp": true, "dist": true, ".idea": true,
	}

	var walk func(dir string, prefix string)
	walk = func(dir string, prefix string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		for i, entry := range entries {
			if ignoreDirs[entry.Name()] {
				continue
			}

			isLast := i == len(entries)-1
			marker := "├── "
			if isLast {
				marker = "└── "
			}

			if entry.IsDir() {
				buf.WriteString(fmt.Sprintf("%s%s%s/\n", prefix, marker, entry.Name()))
				newPrefix := prefix
				if isLast {
					newPrefix += "    "
				} else {
					newPrefix += "│   "
				}
				walk(filepath.Join(dir, entry.Name()), newPrefix)
			} else {
				// Показываем только файлы с нужными расширениями
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if ext == ".go" || ext == ".mod" || ext == ".md" || ext == ".sum" {
					buf.WriteString(fmt.Sprintf("%s%s%s\n", prefix, marker, entry.Name()))
				}
			}
		}
	}

	walk(rootDir, "")
	return buf.String()
}

// processFile обрабатывает один файл и записывает его в выходной файл
func processFile(out *os.File, fp string) error {
	// Читаем содержимое файла
	content, err := os.ReadFile(fp)
	if err != nil {
		return fmt.Errorf("ошибка чтения: %w", err)
	}

	// Определяем язык для подсветки (для markdown-совместимости)
	lang := "go"

	ext := strings.ToLower(filepath.Ext(fp))
	if ext == ".md" {
		lang = "markdown"
	} else if ext == ".mod" || ext == ".sum" {
		lang = "text"
	}

	// Записываем заголовок файла
	header := fmt.Sprintf("\n\n===========================================\n"+
		"ФАЙЛ: %s\n"+
		"===========================================\n"+
		"```%s\n",
		fp, lang)

	if _, err := out.WriteString(header); err != nil {
		return fmt.Errorf("ошибка записи заголовка: %w", err)
	}

	// Записываем содержимое
	if _, err := out.Write(content); err != nil {
		return fmt.Errorf("ошибка записи содержимого: %w", err)
	}

	// Добавляем перенос строки, если его нет
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := out.WriteString("\n"); err != nil {
			return fmt.Errorf("ошибка записи переноса: %w", err)
		}
	}

	// Закрываем блок кода
	if _, err := out.WriteString("```\n"); err != nil {
		return fmt.Errorf("ошибка записи закрытия блока: %w", err)
	}

	return nil
}
