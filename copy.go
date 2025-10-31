package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	log "github.com/schollz/logger"
)

// Тип функции для копирования файла
type CopyFileFunc func(srcURI fyne.URIReadCloser, dstPath string) error

func copyFiles(srcURI fyne.URI, dstDir string, copyFileFn CopyFileFunc) error {

	// Для отслеживания циклов
	visited := make(map[string]bool)
	deep := 0

	var walk func(current fyne.URI, currentRelPath string) error

	// Определяем walk внутри copyFiles, чтобы она имела доступ к visited, dstDir, и copyFileFn
	walk = func(current fyne.URI, currentRelPath string) error {

		currentStr := current.String()

		// Проверяем, посещали ли мы этот URI раньше (защита от циклов)
		// Безопасно, так как мы в GUI потоке Fyne
		if visited[currentStr] {
			log.Tracef("Cycle detected, skipping: %s", currentStr)
			return nil
		}

		var finalRelPath string
		if deep == 0 {
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

		log.Tracef("walk:\ncurrent\t%s\ndstDir\t%s\nrelPath\t%s\ndstPath\t%s\ndeep\t%v", current, dstDir, currentRelPath, dstPath, deep)
		deep++
		defer func() { deep-- }()

		if canList(current) {
			// Добавляем в visited, так как начали обработку каталога.
			visited[currentStr] = true
			log.Tracef("walk: %s is directory", current)

			children, listErr := storage.List(current)
			if listErr != nil {
				return fmt.Errorf("failed to list directory %s (Read failed, List also failed): %v", current, listErr)
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
				if err := walk(child, relPathForChildDir); err != nil {
					return err
				}
			}
			return nil
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
	dstFilePath := dstDir
	if canList(srcURI) {
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			return err
		}
		dstFilePath = filepath.Join(dstDir, uriBase(srcURI))
		return walk(srcURI, "")
	}

	r, err := storage.Reader(srcURI)
	if err != nil {
		return err
	}

	log.Tracef("copyFileFn %s->%s", r.URI(), dstFilePath)
	return copyFileFn(r, dstFilePath)
}

func canList(u fyne.URI) bool {
	sure := func() bool {
		if can, err := storage.CanList(u); err == nil && can {
			if _, err := storage.List(u); err == nil {
				return true
			}
		}
		return false
	}
	// Проверка MIME-типа для Android
	if MimeType(u) == "vnd.android.document/directory" {
		if sure() {
			return true
		}
	}

	if can, err := storage.CanRead(u); err == nil && !can {
		if sure() {
			return true
		}
	}
	r, err := storage.Reader(u)
	if err != nil {
		if sure() {
			return true
		}
	}
	defer r.Close()

	p := make([]byte, 1)
	_, err = r.Read(p)

	if eIsDir(err) {
		if sure() {
			return true
		}
	}
	return false
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
