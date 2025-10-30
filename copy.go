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
// 1. storage.Reader(uri) не удался.
// 2. storage.Reader(uri) удался, НО storage.Read() не удался с ошибкой, характерной для каталога .
// Это позволяет избежать паники на Android 9 при вызове List на файле.
func copyFiles0(srcURI fyne.URI, dstDir string, copyFileFn CopyFileFunc) error {
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		return err
	}

	// Инициализируем карту visited для отслеживания циклов в рамках этого вызова
	visited := make(map[string]bool)

	var walk func(current fyne.URI, currentRelPath string, isFirst bool) error

	// Определяем walk внутри copyFiles, чтобы она имела доступ к visited, dstDir, и copyFileFn
	walk = func(current fyne.URI, currentRelPath string, isFirst bool) error {
		currentStr := current.String()

		// Проверяем, посещали ли мы этот URI раньше (защита от циклов)
		// Безопасно, так как мы в GUI потоке Fyne
		if visited[currentStr] {
			log.Tracef("Cycle detected, skipping: %s", currentStr)
			return nil
		}

		var finalRelPath string
		if isFirst {
			finalRelPath = currentRelPath
		} else {
			finalRelPath = filepath.Join(currentRelPath, uriBase(current))
		}

		var dstPath string
		if finalRelPath == "" {
			dstPath = dstDir
		} else {
			dstPath = filepath.Join(dstDir, finalRelPath)
		}

		// log.Tracef("storageWalkDir current %s dstDir %s relPath %s dstPath %s isFirst %v", current, dstDir, currentRelPath, dstPath, isFirst)

		// --- Определение типа элемента и обработка ---
		var err error
		var r fyne.URIReadCloser
		if isAndroid {
			mimeType := MimeType(current)
			log.Tracef("URI (%s) has MimeType: %s %s", current, mimeType, current.MimeType())
			if mimeType == "vnd.android.document/directory" {
				err = fmt.Errorf("vnd.android.document/directory")
			}
		}
		if err == nil {
			r, err = storage.Reader(current)
		}
		if err != nil {
			// Добавляем в visited, так как начали обработку каталога.
			visited[currentStr] = true
			log.Tracef("walk: %s is directory error: %v. Marked as visited.", current, err)

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

		peekBuf := make([]byte, 1)
		_, peekErr := r.Read(peekBuf)
		r.Close() // Обязательно закрываем после проверки
		// Проверим, можно ли прочитать.

		if peekErr != nil {
			// Проверим, является ли ошибка признаком каталога.
			if errors.Is(peekErr, syscall.EISDIR) || (peekErr != nil && strings.Contains(peekErr.Error(), "Incorrect function.")) {
				log.Tracef("walk: Reader succ, Read error suggests directory for %s: %v", current, peekErr)

				// Добавляем в visited, так как начали обработку каталога.
				visited[currentStr] = true
				log.Tracef("walk: Marked directory (Reader succ, Read err-dir) as visited: %s", currentStr)

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

	// Проверяем srcURI сначала, чтобы определить, файл это или каталог, до запуска рекурсии.
	var err error
	var r fyne.URIReadCloser
	if isAndroid {
		mimeType := MimeType(srcURI)
		log.Tracef("URI (%s) has MimeType: %s %s", srcURI, mimeType, srcURI.MimeType())
		if mimeType == "vnd.android.document/directory" {
			err = fmt.Errorf("vnd.android.document/directory")
		}
	}
	if err == nil {
		r, err = storage.Reader(srcURI)
	}
	if err != nil {
		// Reader для srcURI не удался -> это каталог.
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
			// Reader succeeded, Read failed как каталог -> это каталог.
			// Запускаем walk, которая попытается вызвать List, что теперь считается безопасным.
			// isFirst = true
			return walk(srcURI, "", true)
		}
		// Reader succeeded, Read failed как файл -> ошибка.
		return fmt.Errorf("failed to read initial file URI %s: %v", srcURI, peekErr)
	}

	// Reader succeeded, Read succeeded -> это файл.
	dstFilePath := filepath.Join(dstDir, uriBase(srcURI))
	r, err = storage.Reader(srcURI) // Открываем снова для копирования
	if err != nil {
		return err
	}
	return copyFileFn(r, dstFilePath)
}

func eIsDir(err error) bool {
	if err == nil {
		return false
	}

	// Проверка Unix ошибки
	if errors.Is(err, syscall.EISDIR) {
		return true
	}

	// Проверка Windows ошибки через Errno
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == 1 { // ERROR_INCORRECT_FUNCTION
		return true
	}

	// Проверка текста ошибки для всех платформ
	// errStr := strings.ToLower(err.Error())
	// if strings.Contains(errStr, "incorrect function") ||
	// 	strings.Contains(errStr, "is a directory") {
	// 	return true
	// }

	return false
}

func canList(u fyne.URI) bool {
	// Проверка MIME-типа для Android
	if MimeType(u) == "vnd.android.document/directory" {
		return true
	}

	r, err := storage.Reader(u)
	if err != nil {
		return true // Не удалось открыть как файл -> вероятно каталог
	}
	defer r.Close()

	p := make([]byte, 1)
	_, err = r.Read(p)

	return eIsDir(err) // Используйте вашу функцию eIsDir для кроссплатформенной проверки
}

func copyFiles(srcURI fyne.URI, dstDir string, copyFileFn CopyFileFunc) error {
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		return err
	}

	// Инициализируем карту visited для отслеживания циклов в рамках этого вызова
	visited := make(map[string]bool)

	var walk func(current fyne.URI, currentRelPath string, isFirst bool) error

	// Определяем walk внутри copyFiles, чтобы она имела доступ к visited, dstDir, и copyFileFn
	walk = func(current fyne.URI, currentRelPath string, isFirst bool) error {
		currentStr := current.String()

		// Проверяем, посещали ли мы этот URI раньше (защита от циклов)
		// Безопасно, так как мы в GUI потоке Fyne
		if visited[currentStr] {
			log.Tracef("Cycle detected, skipping: %s", currentStr)
			return nil
		}

		var finalRelPath string
		if isFirst {
			finalRelPath = currentRelPath
		} else {
			finalRelPath = filepath.Join(currentRelPath, uriBase(current))
		}

		var dstPath string
		if finalRelPath == "" {
			dstPath = dstDir
		} else {
			dstPath = filepath.Join(dstDir, finalRelPath)
		}

		log.Tracef("copyFiles current %s dstDir %s relPath %s dstPath %s isFirst %v", current, dstDir, currentRelPath, dstPath, isFirst)

		// --- Определение типа элемента и обработка ---
		if canList(current) {
			// Добавляем в visited, так как начали обработку каталога.
			visited[currentStr] = true
			log.Tracef("walk: %s is directory", current)

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

		// Файлы не добавляются в visited (они не образуют циклов при рекурсивном обходе).
		// Убедимся, что родительский каталог существует.
		dir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}

		r, err := storage.Reader(current)
		if err != nil {
			return err
		}

		log.Tracef("walk: copyFileFn %s->%s", r.URI(), dstPath)
		return copyFileFn(r, dstPath)
	}

	// Проверяем srcURI сначала, чтобы определить, файл это или каталог, до запуска рекурсии.
	if canList(srcURI) {
		return walk(srcURI, "", true)
	}

	r, err := storage.Reader(srcURI)
	if err != nil {
		return err
	}

	dstFilePath := filepath.Join(dstDir, uriBase(srcURI))
	log.Tracef("copyFileFn %s->%s", r.URI(), dstFilePath)
	return copyFileFn(r, dstFilePath)
}
