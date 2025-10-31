//go:build !android

package main

import (
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
