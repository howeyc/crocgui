//go:build windows

package main

import (
	"os"
	"syscall"
)

// CanCreateSymlinks проверяет, запущено ли приложение с правами администратора.
func CanCreateSymlinks() bool {
	if f, err := os.Open("\\\\.\\PHYSICALDRIVE0"); err == nil {
		f.Close()
		return true
	}
	return false
}

func isGUIApplication() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	h, _, _ := getConsoleWindow.Call()
	return h == 0
}
