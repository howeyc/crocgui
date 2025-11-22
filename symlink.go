// symlink.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/schollz/logger"
)

// Symlink создает симлинк или псевдосимлинк в зависимости от CanCreateSymlinks.
func Symlink(oldname string, newname string) error {
	if isMobile || asMobile {
		return fmt.Errorf("isMobile")
	}
	if swap {
		return fmt.Errorf("swap")
	}
	// Создаем директорию для новой ссылки если нужно
	dir := filepath.Dir(newname)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	if CanCreateSymlinks() {
		// Пробуем создать настоящий симлинк
		return os.Symlink(oldname, newname)
	}

	// Создаем файл-псевдосимлинк с содержимым "→oldname"
	content := PSL + oldname
	return os.WriteFile(newname, []byte(content), 0644)
}

// Readlink читает симлинк или псевдосимлинк.
// Если это не симлинк - возвращает ошибку
func Readlink(name string) (string, error) {
	if isMobile {
		return "", fmt.Errorf("isMobile")
	}
	// Пробуем прочитать как настоящий симлинк
	fileInfo, err := os.Lstat(name)
	if err != nil {
		return "", err
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return os.Readlink(name)
	}

	// Пробуем прочитать как псевдосимлинк
	contentBytes, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(string(contentBytes))
	if !strings.HasPrefix(content, PSL) || len(content) <= len(PSL) {
		return "", os.ErrInvalid
	}

	target := strings.TrimSpace(content[len(PSL):])
	if target == "" {
		return "", os.ErrInvalid
	}

	// Проверяем существование цели для псевдосимлинков
	if _, err := os.Stat(target); err != nil {
		// Пробуем относительный путь
		if !filepath.IsAbs(target) {
			absTarget := filepath.Join(filepath.Dir(name), target)
			if _, err := os.Stat(absTarget); err == nil {
				return target, nil
			}
		}
		return "", os.ErrNotExist
	}

	return target, nil
}

func isLinkDir(path string) (ok bool) {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return true
	}
	if target, err := Readlink(path); err == nil {
		log.Tracef("is symlink %s to %s", path, target)
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			return true
		}
	}
	return
}

func hasFolder(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			return true
		}

		file := filepath.Join(path, entry.Name())
		if target, err := Readlink(file); err == nil {
			fi, err := os.Stat(target)
			if err == nil && fi.IsDir() {
				return true
			}
		}
	}
	return false
}

func isEmptyFolder(folderPath string) (bool, error) {
	f, err := os.Open(folderPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, nil
}
