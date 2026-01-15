package main

import (
	"fyne.io/fyne/v2/lang"
	"golang.org/x/text/message"
)

var langCode = lang.SystemLocale().String()
var langPrinter *message.Printer

// lp uses langPrinter to output the string in selected language
func lp(s string) string {
	return langPrinter.Sprintf(s)
}
