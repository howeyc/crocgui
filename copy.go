package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	log "github.com/schollz/logger"
)

// Тип функции для копирования файла
type CopyFileFunc func(srcURI fyne.URIReadCloser, dstPath string) error

// copyFiles копирует файл или каталог (и его содержимое рекурсивно) из srcURI в dstDir.
// Предполагается, что вызывается из GUI-горутины Fyne для обеспечения безопасности доступа к внутренней карте visited.
// ВНИМАНИЕ: Использует storage.Reader + Read для определения типа файла/каталога.
// storage.List вызывается ТОЛЬКО если:
// 1. storage.Reader(uri) не удался (обычно каталог, Android 9).
// 2. storage.Reader(uri) удался, НО storage.Read() не удался с ошибкой, характерной для каталога (Android 14).
// Это позволяет избежать паники на Android 9 при вызове List на файле, где Reader succeeded.
func copyFiles(srcURI fyne.URI, dstDir string, copyFileFn CopyFileFunc) error {
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		return err
	}

	log.Tracef("copyFiles: Начало обработки srcURI: %s, dstDir: %s", srcURI, dstDir)

	// Инициализируем карту visited для отслеживания циклов в рамках этого вызова
	visited := make(map[string]bool)
	log.Tracef("copyFiles: Инициализирована пустая карта visited. Длина: 0", len(visited))

	var walk func(current fyne.URI, currentRelPath string, isFirst bool) error

	// Определяем walk внутри copyFiles, чтобы она имела доступ к visited, dstDir, и copyFileFn
	walk = func(current fyne.URI, currentRelPath string, isFirst bool) error {
		currentPathStr := current.Path()

		// Проверяем, посещали ли мы этот URI раньше (защита от циклов)
		// Безопасно, так как мы в GUI потоке Fyne
		if visited[currentPathStr] {
			log.Tracef("Cycle detected, skipping: %s", currentPathStr)
			return nil
		}

		name := uriBase(current)
		var finalRelPath string
		if isFirst {
			finalRelPath = currentRelPath
		} else {
			finalRelPath = filepath.Join(currentRelPath, name)
		}

		var dstPath string
		if finalRelPath == "" {
			dstPath = dstDir
		} else {
			dstPath = filepath.Join(dstDir, finalRelPath)
		}

		log.Tracef("storageWalkDir current %s dstDir %s relPath %s dstPath %s isFirst %v", current, dstDir, currentRelPath, dstPath, isFirst)

		// --- Определение типа элемента и обработка ---
		r, err := storage.Reader(current)
		if err != nil {
			// Reader не удался -> это каталог (ожидаемо для Android 9).
			// Добавляем в visited, так как начали обработку каталога.
			visited[currentPathStr] = true
			log.Tracef("walk: Reader error for %s: %v (likely directory). Marked as visited.", current, err)

			// Вызов List безопасен, так как Reader не удался.
			children, listErr := storage.List(current)
			if listErr != nil {
				return fmt.Errorf("failed to list directory %s (Reader failed, List also failed): %v", current, listErr)
			}

			log.Tracef("walk: List for %s returned %d items", current, len(children))

			// Вычисляем relPath для дочерних элементов относительно dstDir
			relPathForChildDir, errRel := filepath.Rel(dstDir, dstPath)
			if errRel != nil {
				return fmt.Errorf("failed to compute relative path for child dir %s: %v", dstPath, errRel)
			}

			for _, child := range children {
				if child.String() == current.String() {
					log.Tracef("walk: Skipping child that matches parent URI: %s", child)
					continue
				}
				// isFirst = false для дочерних элементов
				if err := walk(child, relPathForChildDir, false); err != nil {
					return err
				}
			}
			return nil // Успешно обработан каталог
		}

		// Reader succeeded -> это НЕ каталог (ожидаемо для Android 9), но может быть каталогом (Android 14).
		// Проверим, можно ли прочитать.
		peekBuf := make([]byte, 1)
		_, peekErr := r.Read(peekBuf)
		r.Close() // Обязательно закрываем после проверки

		if peekErr != nil {
			// Reader succeeded, но Read failed.
			// Проверим, является ли ошибка признаком каталога (Android 14).
			if errors.Is(peekErr, syscall.EISDIR) || (peekErr != nil && strings.Contains(peekErr.Error(), "Incorrect function.")) {
				// Read error указывает на каталог, несмотря на Reader success (Android 14).
				// Это означает, что 'current' - это каталог.
				// Теперь безопасно вызвать List, так как мы уверены, что это не файл (где Reader succeeded).
				log.Tracef("walk: Reader succ, Read error suggests directory for %s: %v", current, peekErr)

				// Добавляем в visited, так как начали обработку каталога.
				visited[currentPathStr] = true
				log.Tracef("walk: Marked directory (Reader succ, Read err-dir) as visited: %s", currentPathStr)

				// Вызов List безопасен, так как Read дал ошибку каталога, подтверждая тип.
				children, listErr := storage.List(current)
				if listErr != nil {
					return fmt.Errorf("failed to list directory %s (Reader succ, Read err-dir, List failed): %v", current, listErr)
				}

				log.Tracef("walk: List (after Read error) for %s returned %d items", current, len(children))

				relPathForChildDir, errRel := filepath.Rel(dstDir, dstPath)
				if errRel != nil {
					return fmt.Errorf("failed to compute relative path for child dir %s: %v", dstPath, errRel)
				}

				for _, child := range children {
					if child.String() == current.String() {
						log.Tracef("walk: Skipping child that matches parent URI: %s", child)
						continue
					}
					// isFirst = false для дочерних элементов
					if err := walk(child, relPathForChildDir, false); err != nil {
						return err
					}
				}
				return nil // Успешно обработан каталог после Read ошибки

			} else {
				// Reader succeeded, Read failed, но ошибка НЕ указывает на каталог.
				// Это ошибка файла.
				return fmt.Errorf("failed to read from file URI %s (Reader succ, Read failed as file): %v", current, peekErr)
			}
		}

		// Reader succeeded, Read succeeded (даже 0 байт). Это файл.
		// Файлы не добавляются в visited (они не образуют циклов при рекурсивном обходе).
		// Убедимся, что родительский каталог существует.
		dir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}

		// Повторно получаем Reader для копирования
		r, err = storage.Reader(current)
		if err != nil {
			return err
		}
		log.Tracef("walk: copyFileFn %s->%s", r.URI(), dstPath)
		return copyFileFn(r, dstPath)
	}

	// --- ВНЕШНЯЯ ПРОВЕРКА ДЛЯ srcURI ---
	// Проверяем srcURI сначала, чтобы определить, файл это или каталог, до запуска рекурсии.
	r, err := storage.Reader(srcURI)
	if err != nil {
		// Reader для srcURI не удался -> это каталог (Android 9).
		// Запускаем walk, которая сразу вызовет List.
		// isFirst = true
		return walk(srcURI, "", true)
	}

	// Reader для srcURI succeeded.
	peekBuf := make([]byte, 1)
	_, peekErr := r.Read(peekBuf)
	r.Close() // Обязательно закрываем после проверки

	if peekErr != nil {
		if errors.Is(peekErr, syscall.EISDIR) || (peekErr != nil && strings.Contains(peekErr.Error(), "Incorrect function.")) {
			// Reader succeeded, Read failed как каталог -> это каталог (Android 14).
			// Запускаем walk, которая попытается вызвать List, что теперь считается безопасным.
			// isFirst = true
			return walk(srcURI, "", true)
		}
		// Reader succeeded, Read failed как файл -> ошибка.
		return fmt.Errorf("failed to read initial file URI %s: %v", srcURI, peekErr)
	}

	// Reader succeeded, Read succeeded -> это файл.
	// Копируем его.
	fileName := uriBase(srcURI)
	dstFilePath := filepath.Join(dstDir, fileName)
	r, err = storage.Reader(srcURI) // Открываем снова для копирования
	if err != nil {
		return err
	}
	return copyFileFn(r, dstFilePath)
}
