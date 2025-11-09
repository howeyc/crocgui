//go:build !android

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

func processIntent() {}

func uriBase(uri fyne.URI) string {
	return uri.Name()
}

func IsFilePickerSupported() (bool, error)   { return !noDialogDebug, nil }
func IsSaveDialogSupported() (bool, error)   { return !noDialogDebug, nil }
func IsFolderPickerSupported() (bool, error) { return !noDialogDebug, nil }

func RequestStoragePermission() {}
func OpenAppSettings()          {}

func CanList(u fyne.URI) (bool, error) {
	return storage.CanList(u)
}

func IsDirectory(u fyne.URI) (ok bool) {
	if u == nil {
		return false
	}
	ok, _ = storage.CanList(u)
	return
}

func MimeType(u fyne.URI) string { return u.MimeType() }
func apiLevel() int              { return 29 }

func sendNotification(a fyne.App, title, content string) {
	// Стандартное уведомление для других платформ
	notification := fyne.NewNotification(title, content)
	a.SendNotification(notification)
}

func LogD(string)         {}
func excludeFromRecents() {}

func hasChild(uri fyne.URI) (isDir bool, childCount int, err error) {
	return storageChild(uri)
}

// getSize возвращает размер файла в байтах для не-Android платформ
func getSize(uri fyne.URI) (size int64, err error) {
	if uri == nil {
		return 0, fmt.Errorf("uri is nil")
	}

	// Проверяем существование файла
	exists, err := storage.Exists(uri)
	if err != nil {
		return 0, fmt.Errorf("failed to check if URI exists: %w", err)
	}
	if !exists {
		return 0, fmt.Errorf("file does not exist: %s", uri.String())
	}

	// Пытаемся получить размер через os.Stat для локальных файлов
	if uri.Scheme() == "file" || uri.Scheme() == "" {
		filePath := uri.Path()
		fileInfo, err := os.Stat(filePath)
		if err == nil {
			return fileInfo.Size(), nil
		}
		// Если os.Stat не сработал, продолжаем другими методами
	}

	// Альтернативный метод: открываем и читаем файл для определения размера
	readCloser, err := storage.OpenFileFromURI(uri)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer readCloser.Close()

	written, err := io.Copy(io.Discard, readCloser)
	if err != nil {
		return 0, fmt.Errorf("failed to read file content: %w", err)
	}

	return written, nil
}

func start() {
	cmd := exec.Command(os.Args[0])
	cmd.Env = os.Environ()
	cmd.Dir = wd
	cmd.Start()
}

func List(u fyne.URI) (c []fyne.URI, err error) {
	if u == nil {
		err = fmt.Errorf("uri is nul")
		return
	}
	return storage.List(u)
}

func Reader(u fyne.URI) (r fyne.URIReadCloser, err error) {
	if u == nil {
		err = fmt.Errorf("uri is nul")
		return
	}
	return storage.Reader(u)
}
