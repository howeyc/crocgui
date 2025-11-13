//go:build !android && !ios

// child_desktop.go
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
		err = fmt.Errorf("failed to get Downloads directory")
		return
	}

	u := storage.NewFileURI(downloads)
	lu, err := storage.ListerForURI(u)
	if err != nil {
		err = fmt.Errorf("create lister for %s: %v", u, err)
		return
	}

	child, err = storage.Child(lu, component)
	return
}

func Parent(child fyne.URI) (parent fyne.ListableURI, err error) {
	pu, err := storage.Parent(child)
	if err != nil {
		return nil, fmt.Errorf("get parent URI: %v", err)
	}

	parent, err = storage.ListerForURI(pu)
	if err != nil {
		return nil, fmt.Errorf("make listable: %v", err)
	}

	return parent, nil
}
