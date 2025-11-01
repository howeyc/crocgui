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

		ok, err := canList(current)
		if ok {
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
		if err != nil {
			return err
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
	ok, err := canList(srcURI)
	if ok {
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			return err
		}
		dstFilePath = filepath.Join(dstDir, uriBase(srcURI))
		return walk(srcURI, "")
	}
	if err != nil {
		return err
	}

	r, err := storage.Reader(srcURI)
	if err != nil {
		return err
	}

	log.Tracef("copyFileFn %s->%s", r.URI(), dstFilePath)
	return copyFileFn(r, dstFilePath)
}

func canList(u fyne.URI) (bool, error) {
	sure := func() bool {
		if can, err := storage.CanList(u); err == nil && can {
			log.Trace("CanList")
			if items, err := storage.List(u); err == nil {
				log.Tracef("List %v", items)
				return true
			} else {
				log.Errorf("List error: %v", err)
			}
		}
		return false
	}
	// Проверка MIME-типа для Android
	if m := MimeType(u); m == "vnd.android.document/directory" {
		log.Tracef("MimeType %s", m)
		if sure() {
			return true, nil
		}
	}

	if can, err := storage.CanRead(u); err == nil && !can {
		log.Trace("!CanRead")
		if sure() {
			return true, nil
		}
	}

	r, err := storage.Reader(u)
	if err != nil {
		log.Errorf("Reader error: %v", err)
		if sure() {
			return true, nil
		}
	}
	defer r.Close()

	p := make([]byte, 1)
	_, err = r.Read(p)
	if err == nil {
		return false, nil
	}
	// ok, err := canRead(u)
	// if err != nil && ok {
	// 	return false, nil
	// }

	if eIsDir(err) {
		log.Trace("eIsDir")
		if sure() {
			return true, nil
		}
	}

	return false, err
}

func canRead(u fyne.URI) (ok bool, err error) {
	ok, err = storage.CanRead(u)
	if err != nil || !ok {
		return
	}
	log.Trace("CanRead")

	r, err := storage.Reader(u)
	if err != nil {
		log.Errorf("Reader error: %v", err)
		return false, err
	}
	defer r.Close()

	p := make([]byte, 1)
	_, err = r.Read(p)
	return err == nil, err
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
