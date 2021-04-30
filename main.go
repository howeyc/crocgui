package main

import (
	"bytes"
	_ "embed"
	"strings"
	"time"

	log "github.com/schollz/logger"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
)

//go:embed metadata/en-US/images/featureGraphic.png
var textlogobytes []byte

type logwriter struct {
	buf        bytes.Buffer
	lastlines  []string
	lastupdate time.Time
}

const LOG_LINES = 20

func (lw *logwriter) Write(p []byte) (n int, err error) {
	n, err = lw.buf.Write(p)

	lw.lastlines = append([]string{string(p)}, lw.lastlines...)
	if len(lw.lastlines) > LOG_LINES {
		lw.lastlines = lw.lastlines[:LOG_LINES]
	}

	if time.Since(lw.lastupdate) > time.Second {
		logbinding.Set(strings.Join(lw.lastlines, ""))
		lw.lastupdate = time.Now()
	}
	return
}

var logoutput logwriter
var logbinding binding.String

func main() {
	a := app.NewWithID("com.github.howeyc.crocgui")
	w := a.NewWindow("croc")

	logbinding = binding.NewString()
	log.SetOutput(&logoutput)

	// Defaults
	a.Preferences().SetString("relay-address", a.Preferences().StringWithFallback("relay-address", "croc.schollz.com:9009"))
	a.Preferences().SetString("relay-password", a.Preferences().StringWithFallback("relay-password", "pass123"))
	a.Preferences().SetString("relay-ports", a.Preferences().StringWithFallback("relay-ports", "9009,9010,9011,9012,9013"))
	a.Preferences().SetBool("disable-local", a.Preferences().BoolWithFallback("disable-local", false))
	a.Preferences().SetBool("force-local", a.Preferences().BoolWithFallback("force-local", false))
	a.Preferences().SetBool("disable-multiplexing", a.Preferences().BoolWithFallback("disable-multiplexing", false))
	a.Preferences().SetBool("disable-compression", a.Preferences().BoolWithFallback("disable-compression", false))
	a.Preferences().SetString("theme", a.Preferences().StringWithFallback("theme", "light"))
	a.Preferences().SetString("debug-level", a.Preferences().StringWithFallback("debug-level", "error"))
	a.Preferences().SetString("pake-curve", a.Preferences().StringWithFallback("pake-curve", "siec"))
	a.Preferences().SetString("croc-hash", a.Preferences().StringWithFallback("croc-hash", "xxhash"))

	setTheme(a.Preferences().String("theme"))
	log.SetLevel(a.Preferences().String("debug-level"))

	textlogores := fyne.NewStaticResource("text-logo", textlogobytes)
	textlogo := canvas.NewImageFromResource(textlogores)
	textlogo.SetMinSize(fyne.NewSize(205, 100))
	top := container.NewHBox(layout.NewSpacer(), textlogo, layout.NewSpacer())
	w.SetContent(container.NewBorder(top, nil, nil, nil,
		container.NewAppTabs(
			sendTabItem(a, w),
			recvTabItem(a, w),
			settingsTabItem(a),
			aboutTabItem(),
		)))
	w.Resize(fyne.NewSize(800, 600))

	setDebugObjects()

	w.ShowAndRun()
}
