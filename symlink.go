// symlink.go
package main

import (
	"fmt"
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
	if fi, _ := os.Stat(path); fi != nil && fi.IsDir() {
		return true
	}
	if target, err := Readlink(path); err == nil {
		log.Tracef("is symlink %s to %s", path, target)
		if fi, _ := os.Stat(target); fi != nil && fi.IsDir() {
			return true
		}
	}
	return
}
