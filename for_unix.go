//go:build unix && !android && !darwin

package main

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
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

	log.Debugf("caffeinate(%d): old=%d, newVal=%d, inhibitorCmd=%v", i, old, newVal, inhibitorCmd != nil)

	// Используем очередь Fyne вместо мьютекса для управления процессом
	fyne.Do(func() {
		// Включение: переход из 0 в зону активности
		if old <= 0 && newVal > 0 && inhibitorCmd == nil {
			// Проверяем, не запущен ли уже процесс systemd-inhibit
			pid := findRunningInhibitor()
			if pid > 0 {
				// Найден уже запущенный процесс, используем его
				log.Infof("Found existing systemd-inhibit process PID=%d, reusing it", pid)
				process, err := os.FindProcess(pid)
				if err == nil {
					inhibitorCmd = &exec.Cmd{Process: process}
					log.Infof("Reusing existing systemd-inhibit process PID=%d", pid)
				} else {
					log.Errorf("Failed to find process PID=%d: %v", pid, err)
					// Если не удалось найти процесс, создаем новый
					createNewInhibitor(inhibitPath)
				}
			} else {
				// Процесс не найден, создаем новый
				createNewInhibitor(inhibitPath)
			}
		} else if old <= 0 && newVal > 0 && inhibitorCmd != nil {
			log.Warnf("BUG: Trying to create new process but inhibitorCmd is already set! PID=%d", inhibitorCmd.Process.Pid)
		}

		// Выключение: возврат в 0 или принудительный сброс
		if old > 0 && newVal <= 0 && inhibitorCmd != nil {
			pid := inhibitorCmd.Process.Pid
			if inhibitorCmd.Process != nil {
				_ = inhibitorCmd.Process.Kill()
				log.Debugf("Killed systemd-inhibit process PID=%d", pid)
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

// findRunningInhibitor ищет уже запущенный процесс systemd-inhibit с аргументом "crocgui transfer in progress"
// Возвращает PID найденного процесса или 0, если не найден
func findRunningInhibitor() int {
	out, err := exec.Command("pgrep", "-f", "systemd-inhibit.*crocgui transfer in progress").Output()
	if err != nil {
		// pgrep может вернуть ошибку если процесс не найден, это нормально
		log.Debugf("pgrep search failed: %v", err)
		return 0
	}
	outStr := strings.TrimSpace(string(out))
	if outStr == "" {
		return 0
	}
	// pgrep может вернуть несколько PID, берем первый
	pids := strings.Fields(outStr)
	if len(pids) == 0 {
		return 0
	}
	pid, err := strconv.Atoi(pids[0])
	if err != nil {
		log.Errorf("Failed to parse PID %s: %v", pids[0], err)
		return 0
	}
	log.Debugf("Found running systemd-inhibit process PID=%d", pid)
	return pid
}

// createNewInhibitor создает новый процесс systemd-inhibit
func createNewInhibitor(path string) {
	log.Debugf("Creating new systemd-inhibit process")
	cmd := exec.Command(path,
		"--what=idle",
		"--why=crocgui transfer in progress",
		"sleep", "infinity")

	if err := cmd.Start(); err == nil {
		log.Debugf("Started systemd-inhibit process PID=%d", cmd.Process.Pid)
		inhibitorCmd = cmd
		// Ждем завершения в фоне, чтобы очистить ресурсы (zombie reaper)
		go func() {
			_ = cmd.Wait()
			log.Debugf("systemd-inhibit process PID=%d exited", cmd.Process.Pid)
		}()
	} else {
		log.Errorf("Start %s: %v", cmd, err)
	}
}
