//go:build !android

package main

import "fyne.io/fyne/v2"

func setupIntentHandler() {}
func uriBase(uri fyne.URI) string {
	return uri.Name()
}
