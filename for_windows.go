//go:build windows

package main

/*
#include <stdlib.h>
#include <stdio.h>

// Обработчик для Stack Protector
void __stack_chk_fail(void) { exit(1); }
void* __stack_chk_guard = NULL;

// Обработчик для Fortify Source
void __chk_fail(void) { exit(1); }
*/
// import "C"

import (
	"fmt"
	"net/url"
	"os/exec"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	log "github.com/schollz/logger"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	setThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

const (
	ES_CONTINUOUS      = 0x80000000
	ES_SYSTEM_REQUIRED = 0x00000001
)

func caffeinate(i int32) int32 {
	old := atomic.LoadInt32(&sleepCounter)
	var newVal int32

	if i == 0 {
		atomic.StoreInt32(&sleepCounter, 0)
		newVal = 0
	} else {
		newVal = atomic.AddInt32(&sleepCounter, i)
	}

	// Оборачиваем системный вызов в очередь Fyne для потокобезопасности
	// и единообразия с другими платформами
	fyne.Do(func() {
		// Состояние изменилось: с 0 на >0 (нужна блокировка)
		if old <= 0 && newVal > 0 {
			setThreadExecutionState.Call(uintptr(ES_CONTINUOUS | ES_SYSTEM_REQUIRED))
		} else if old > 0 && newVal <= 0 {
			// Состояние вернулось в норму (снимаем блокировку)
			setThreadExecutionState.Call(uintptr(ES_CONTINUOUS))
		}
	})

	return newVal
}

func SleepAllowed() bool {
	return atomic.LoadInt32(&sleepCounter) <= 0
}

// CanCreateSymlinks проверяет права администратора.
func CanCreateSymlinks() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func registered(scheme string) bool {
	// Проверяем наличие ключа в реестре: HKEY_CLASSES_ROOT\<scheme>\shell\open\command
	k, err := registry.OpenKey(registry.CLASSES_ROOT, scheme+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	return true
}

// netUse открывает WebDAV ресурс в проводнике Windows
// Примеры:
//
//	netUse("http://127.0.0.1:8080")
//	netUse("http://127.0.0.1:8080/")
//	netUse("https://example.com:8443")
func netUse(u *url.URL, del bool) error {
	// Извлекаем хост и порт
	host := u.Hostname()
	port := u.Port()
	uncPath := fmt.Sprintf(`\\%s@%s\DavWWWRoot`, host, port)
	webdavURL := fmt.Sprintf("%s://%s:%s", u.Scheme, host, port)
	if port == "" {
		uncPath = fmt.Sprintf(`\\%s\DavWWWRoot`, host)
		webdavURL = fmt.Sprintf("%s://%s", u.Scheme, host)
	}

	log.Debugf("UNC Path: %s", uncPath)
	log.Debugf("WebDAV URL: %s", webdavURL)

	// Шаг 1: Удаляем существующее подключение (если есть)
	log.Debug("Removing existing connection...")
	exec.Command("net", "use", uncPath, "/delete").Run()
	if del {
		return nil
	}

	time.Sleep(500 * time.Millisecond)

	// Шаг 2: Подключаем WebDAV
	log.Debug("Connecting WebDAV...")
	cmd := exec.Command("net", "use", uncPath, webdavURL, "/persistent:no")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Debugf("net use output: %s", out)
		return fmt.Errorf("failed to connect WebDAV: %v", err)
	}

	// Шаг 3: Открываем в проводнике
	log.Debug("Opening in explorer...")
	exec.Command("explorer", uncPath).Start()
	return nil
}
