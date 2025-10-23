//go:build windows

package main

import (
	"os"
)

// CanCreateSymlinks проверяет, запущено ли приложение с правами администратора.
func CanCreateSymlinks() bool {
	if f, err := os.Open("\\\\.\\PHYSICALDRIVE0"); err == nil {
		f.Close()
		return true
	}
	return false
}
