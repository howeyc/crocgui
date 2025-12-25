// copy.go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	log "github.com/schollz/logger"
)

func Rename(src, dst string) error {
	if noRename {
		return fmt.Errorf("no rename")
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}

	// Check that dst is not a subdirectory of src
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	if strings.HasPrefix(dstAbs, srcAbs+string(filepath.Separator)) {
		return errors.New("destination cannot be inside source directory")
	}

	// Try standard rename first
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else {
		return err
	}
}

// Тип функции для копирования файла
type CopyFile func(srcURI fyne.URI, dstPath string) error

func copyFiles(srcURI fyne.URI, dstDir string, copyFile CopyFile) error {
	// Для отслеживания циклов
	var visited sync.Map
	deep := 0

	var walk func(current fyne.URI, currentRelPath string) error

	// Определяем walk внутри copyFiles, чтобы она имела доступ к visited, dstDir, и copyFile
	walk = func(current fyne.URI, currentRelPath string) error {
		currentStr := current.String()

		// Проверяем - Load безопасен для конкурентного доступа
		if _, loaded := visited.Load(currentStr); loaded {
			return fmt.Errorf("walk visited %s", currentStr)
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

		// log.Debugf("walk:\ncurrent\t%s\ndstDir\t%s\nrelPath\t%s\ndstPath\t%s\ndeep\t%v", current, dstDir, currentRelPath, dstPath, deep)
		deep++
		defer func() { deep-- }()

		if IsDirectory(current) {
			// Сохраняем - Store безопасен для конкурентного доступа
			visited.Store(currentStr, true)

			if isAndroid && deep > 1 {
				return fmt.Errorf("walk deep %d", deep)
			}

			children, err := List(current)
			if err != nil {
				return fmt.Errorf("walk list %s: %w", current, err)
			}

			count := len(children)
			if count == 0 {
				return fmt.Errorf("count == 0")
			}
			log.Debugf("walk list %s: %d", current, count)

			// Вычисляем relPath для дочерних элементов относительно dstDir
			relPathForChildDir, errRel := filepath.Rel(dstDir, dstPath)
			if errRel != nil {
				return fmt.Errorf("walk rel %s: %w", dstPath, errRel)
			}

			for _, child := range children {
				if child.String() == current.String() {
					log.Debugf("walk skipping %s", child)
					continue
				}
				if err := walk(child, relPathForChildDir); err != nil {
					log.Errorf("walk %s walk %s: %v", current, child, err)
					// return err
					// Продолжаем обработку других детей
				}
			}
			return nil
		}

		// Это файл
		dir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("walk MkdirAll: %w", err)
		}

		if err := copyFile(current, dstPath); err != nil {
			return fmt.Errorf("walk copyFile %s %s: %w", current, dstPath, err)
		} else {
			log.Debugf("walk copyFile %s %s", current, dstPath)
		}

		return nil
	}

	// Проверяем srcURI сначала, чтобы определить, файл это или каталог, до запуска рекурсии.
	if IsDirectory(srcURI) {
		if err := os.MkdirAll(dstDir, 0700); err != nil {
			return fmt.Errorf("mkDirAll: %w", err)
		}
		return walk(srcURI, "")
	}

	log.Debugf("copyFile %s %s", srcURI, dstDir)
	return copyFile(srcURI, dstDir)
}

// func canList(u fyne.URI) bool {
// 	ok, err := storage.CanList(u)
// 	if err != nil {
// 		log.Errorf("CanList error: %v", err)
// 		return false
// 	}
// 	if !ok {
// 		return false
// 	}

// 	log.Debug("CanList")
// 	items, err := storage.List(u)
// 	if err != nil {
// 		log.Errorf("List error: %v", err)
// 		return false
// 	}

// 	log.Debugf("List %d", len(items))
// 	return true
// }

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
	fi, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}

	if !fi.IsDir() {
		return false, 0, nil
	}

	child, err := os.ReadDir(path)
	if err != nil {
		return true, 0, err
	}

	childCount = len(child)
	return true, childCount, nil
}
