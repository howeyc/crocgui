package main

import "golang.org/x/text/message"

var langCode string
var langPrinter *message.Printer

// lp uses langPrinter to output the string in selected language
func lp(s string) string {
	return langPrinter.Sprintf(s)
}
