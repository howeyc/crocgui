//go:build !android && !ios

package main

import (
	"fmt"

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
		err = fmt.Errorf("fail get Downloads")
		return
	}
	u := storage.NewFileURI(downloads)
	lu, err := storage.ListerForURI(u)
	if err != nil {
		err = fmt.Errorf("get ListerForURI %s: %v", u, err)
		return
	}

	child, err = storage.Child(lu, component)
	return
}
