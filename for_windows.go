//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/buildkite/shellwords"
	"golang.org/x/sys/windows"
)

func Symlink(oldname string, newname string) error {
	err := os.Symlink(oldname, newname)
	if err == nil || isAdmin() {
		return err
	}
	if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
		dir := filepath.Dir(newname)
		os.MkdirAll(dir, 0o700)
		return ShellExecute("runas", "cmd", dir, windows.SW_HIDE, "/c", "mklink",
			QuoteArg(filepath.FromSlash(newname)),
			QuoteArg(filepath.FromSlash(oldname)))
	}
	return err
}

func isAdmin() bool {
	if f, err := os.Open("\\\\.\\PHYSICALDRIVE0"); err == nil {
		f.Close()
		return true
	}
	return false
}

func ShellExecute(verb, file, cwd string, showCmd int32, args ...string) error {
	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	filePtr, _ := syscall.UTF16PtrFromString(file)
	argPtr, _ := syscall.UTF16PtrFromString(strings.Join(args, " "))
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)

	return windows.ShellExecute(0, verbPtr, filePtr, argPtr, cwdPtr, showCmd)
}

func QuoteArg(arg string) string {
	if arg == "" {
		return `""`
	}

	isAlreadyQuoted := len(arg) >= 2 && arg[0] == '"' && arg[len(arg)-1] == '"'

	if isAlreadyQuoted || !needsQuoting(arg) {
		return arg
	}

	return shellwords.QuoteBatch(arg)
}

func needsQuoting(arg string) bool {
	if arg == "" {
		return true
	}

	command := "cmd " + arg
	args, err := windows.DecomposeCommandLine(command)
	if err != nil || len(args) != 2 || args[1] != arg {
		return true
	}

	return false
}
