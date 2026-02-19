//go:build unix && !android && !darwin

package main

import (
	"bytes"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	log "github.com/schollz/logger"
)

var (
	inhibitorCmd *exec.Cmd

	onceSystemdInhibit sync.Once
	inhibitPath        string
)

// findInhibitor ищет путь к утилите один раз при первом вызове
func findInhibitor() {
	var err error
	inhibitPath, err = exec.LookPath("systemd-inhibit")
	log.Debugf("LookPath %s: %v", inhibitPath, err)
}

func caffeinate(i int32) int32 {
	old := atomic.LoadInt32(&sleepCounter)
	var newVal int32

	// Логика изменения счетчика
	if i == 0 {
		atomic.StoreInt32(&sleepCounter, 0)
		newVal = 0
	} else {
		newVal = atomic.AddInt32(&sleepCounter, i)
	}

	// Ленивый поиск пути
	onceSystemdInhibit.Do(findInhibitor)
	if inhibitPath == "" {
		return newVal
	}

	// Используем очередь Fyne вместо мьютекса для управления процессом
	fyne.Do(func() {
		// Включение: переход из 0 в зону активности
		if old <= 0 && newVal > 0 && inhibitorCmd == nil {
			cmd := exec.Command(inhibitPath,
				"--what=idle",
				"--why=crocgui transfer in progress",
				"sleep", "infinity")

			if err := cmd.Start(); err == nil {
				log.Debugf("Start %s", cmd)
				inhibitorCmd = cmd
				// Ждем завершения в фоне, чтобы очистить ресурсы (zombie reaper)
				go func() {
					_ = cmd.Wait()
				}()
			} else {
				log.Errorf("Start %s: %v", cmd, err)
			}
		}

		// Выключение: возврат в 0 или принудительный сброс
		if old > 0 && newVal <= 0 && inhibitorCmd != nil {
			if inhibitorCmd.Process != nil {
				_ = inhibitorCmd.Process.Kill()
			}
			inhibitorCmd = nil
		}
	})

	return newVal
}

func SleepAllowed() bool {
	return atomic.LoadInt32(&sleepCounter) <= 0
}

func CanCreateSymlinks() bool {
	return true
}

func registered(scheme string) bool {
	// xdg-mime query default x-scheme-handler/dav
	out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/"+scheme).Output()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return false
	}
	return true
}

func registerScheme(scheme string) error {
	// Получаем текущий обработчик для папок
	cmd := exec.Command("xdg-mime", "query", "default", "inode/directory")
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	defaultHandler := strings.TrimSpace(string(out))
	if defaultHandler == "" {
		return nil // Нет обработчика
	}

	log.Debugf("Default folder handler: %s", defaultHandler)

	// Регистрируем этот же обработчик для scheme://
	cmd = exec.Command("xdg-mime", "default", defaultHandler, "x-scheme-handler/"+scheme)
	if err := cmd.Run(); err != nil {
		return err
	}

	log.Debugf("Registered %s with %s", scheme, defaultHandler)
	return nil
}
