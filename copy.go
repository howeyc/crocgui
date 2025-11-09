// copy.go
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
			return fmt.Errorf("Cycle detected, skipping: %s", currentStr)
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

		if IsDirectory(current) {
			// Добавляем в visited, так как начали обработку каталога.
			visited[currentStr] = true
			if isAndroid && deep > 1 {
				return fmt.Errorf("isAndroid && deep > 1")
			}

			children, err := List(current)
			if err != nil {
				return fmt.Errorf("failed to list directory %s: %v", current, err)
			}

			count := len(children)
			if count == 0 {
				return fmt.Errorf("count == 0")
			}
			log.Tracef("walk: List for %s returned %d items", current, count)

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
					log.Errorf("Failed to process child %s of %s: %v", child, current, err)
					// return err
					// Продолжаем обработку других детей
				}
			}
			return nil
		}

		// Это файл
		dir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("MkdirAll %w", err)
		}

		r, err := Reader(current)
		if err != nil {
			// log.Errorf("Failed to open reader for file %s: %v", current, err)
			return fmt.Errorf("Failed to open reader for file %s: %w", current, err)
		}
		log.Tracef("walk: copyFileFn %s->%s", r.URI(), dstPath)
		copyErr := copyFileFn(r, dstPath)
		if copyErr != nil {
			return fmt.Errorf("Failed to copy file %s to %s: %v", r.URI(), dstPath, copyErr)
		}

		// Копирование прошло успешно
		return nil
	}

	// Проверяем srcURI сначала, чтобы определить, файл это или каталог, до запуска рекурсии.
	if IsDirectory(srcURI) {
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			return err
		}
		return walk(srcURI, "")
	}

	r, err := Reader(srcURI)
	if err != nil {
		return fmt.Errorf("Failed to open reader for file %s: %w", srcURI, err)
	}

	log.Tracef("copyFileFn %s->%s", r.URI(), dstDir)
	return copyFileFn(r, dstDir)
}

func canList(u fyne.URI) bool {
	ok, err := storage.CanList(u)
	if err != nil {
		log.Errorf("CanList error: %v", err)
		return false
	}
	if !ok {
		return false
	}

	log.Trace("CanList")
	items, err := storage.List(u)
	if err != nil {
		log.Errorf("List error: %v", err)
		return false
	}

	log.Tracef("List %d", len(items))
	return true
}

func canRead(uri fyne.URI) bool {
	if uri == nil {
		return false
	}
	switch MimeType(uri) {
	case MIME_TYPE_DIR:
		return false
	case MIME_TYPE_OCTET_STREAM:
		if strings.HasPrefix(uri.String(), ZhangHai) {
			size, sizeErr := getSize(uri)
			if sizeErr == nil && size == 4096 {
				return false // иначе storage.CanRead  вернёт syscall.EISDIR и крэшит
			}
		}
	}
	ok, err := storage.CanRead(uri)
	if err != nil {
		log.Errorf("CanRead error: %v", err)
		return false
	}
	if !ok {
		return false
	}
	log.Trace("CanRead")

	r, err := storage.Reader(uri)
	if err != nil {
		log.Errorf("Reader error: %v", err)
		return false
	}
	defer r.Close()

	p := make([]byte, 1)
	_, err = r.Read(p)
	if err != nil {
		log.Errorf("Read error: %v", err)
		return false
	}
	return true
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

	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "eisdir") ||
		strings.Contains(errStr, "is a directory") ||
		strings.Contains(errStr, "incorrect function") {
		return true
	}

	return false
}

func storageChild(uri fyne.URI) (isDir bool, childCount int, err error) {
	if uri == nil {
		return false, 0, fmt.Errorf("uri is nil")
	}

	isDir, err = storage.CanList(uri)
	if err != nil || !isDir {
		return
	}

	children, err := storage.List(uri)
	childCount = len(children)
	return
}

func fileChild(path string) (isDir bool, childCount int, err error) {
	stat, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}

	if !stat.IsDir() {
		return false, 0, nil
	}

	child, err := os.ReadDir(path)
	if err != nil {
		return true, 0, err
	}

	childCount = len(child)
	return true, childCount, nil
}
