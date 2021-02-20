package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

//go:embed metadata/en-US/images/featureGraphic.png
var textlogobytes []byte

func main() {
	a := app.NewWithID("com.github.howeyc.crocgui")
	w := a.NewWindow("croc")

	a.Preferences().StringWithFallback("relay-address", "croc.schollz.com:9009")
	a.Preferences().StringWithFallback("relay-password", "pass123")
	a.Preferences().StringWithFallback("relay-ports", "9009,9010,9011,9012,9013")

	textlogores := fyne.NewStaticResource("text-logo", textlogobytes)
	textlogo := canvas.NewImageFromResource(textlogores)
	textlogo.SetMinSize(fyne.NewSize(205, 100))
	top := container.NewHBox(layout.NewSpacer(), textlogo, layout.NewSpacer())
	w.SetContent(container.NewBorder(top, nil, nil, nil,
		container.NewAppTabs(
			sendTabItem(a, w),
			recvTabItem(a),
			settingsTabItem(a),
			aboutTabItem(),
		)))
	w.Resize(fyne.NewSize(800, 600))
	w.ShowAndRun()
}
