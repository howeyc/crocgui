package main

import (
	_ "embed"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func parseURL(s string) *url.URL {
	link, _ := url.Parse(s)
	return link
}

//go:embed metadata/en-US/full_description.txt
var longdesc string

func aboutTabItem() *container.TabItem {
	aboutInfo := widget.NewLabel(longdesc)
	aboutInfo.Wrapping = fyne.TextWrapWord
	return container.NewTabItemWithIcon("About", theme.InfoIcon(), container.NewBorder(nil,
		widget.NewForm(
			widget.NewFormItem("croc GUI", widget.NewHyperlink("v1.0.0", parseURL("https://github.com/howeyc/crocgui"))),
			widget.NewFormItem("croc", widget.NewHyperlink("v8.6.7", parseURL("https://github.com/schollz/croc"))),
		),
		nil,
		nil,
		aboutInfo,
	))
}
