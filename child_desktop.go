//go:build !android && !ios

// child_desktop.go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"github.com/adrg/xdg"
)

func Child(parent fyne.URI, component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}
	child, err = storage.Child(parent, component)
	return
}

func ChildDownload(component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	downloads := xdg.UserDirs.Download
	if downloads == "" {
		err = fmt.Errorf("failed to get Downloads directory")
		return
	}

	u := storage.NewFileURI(downloads)

	dirPath := filepath.Dir(component)

	// Проверяем, есть ли реальные поддиректории
	hasSubdirs := dirPath != "." && dirPath != string(filepath.Separator)

	// Создаем полный путь к файлу
	if hasSubdirs {
		dirToCreate := filepath.Join(downloads, dirPath)
		err = os.MkdirAll(dirToCreate, 0755)
		if err != nil {
			err = fmt.Errorf("failed to create directory %s: %v", dirToCreate, err)
			return
		}
		u = storage.NewFileURI(dirToCreate)
	}
	lu, err := storage.ListerForURI(u)
	if err != nil {
		err = fmt.Errorf("create lister for %s: %v", u, err)
		return
	}

	child, err = storage.Child(lu, filepath.Base(component))
	return
}
