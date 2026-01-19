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

// func isWindowsGUI() (ok bool) {
// 	// return syscall.Stdout == 0 && syscall.Stderr == 0
// 	kernel32 := syscall.NewLazyDLL("kernel32.dll")
// 	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
// 	h, _, _ := getConsoleWindow.Call()
// 	ok = h == 0
// 	if ok {
// 		WR, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
// 		if err == nil {
// 			os.Stderr = WR
// 			os.Stdout = WR
// 		}
// 		RD, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
// 		if err != nil {
// 			os.Stdin = RD
// 		}
// 	}
// 	return
// }
