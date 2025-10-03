package main

import (
	"embed"
	"fmt"
	"strings"

	log "github.com/schollz/logger"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	// Чтоб на десктопе отладить копирование вместо переноса как будто это мобильная ОС.
	copyDebug = false
	// Чтоб на десктопе или Андроиде 9- отладить план Б при отсутствии com.android.DocumentsUI на мобильной ОС сохранять протокол и полученные файлы в Загрузки.
	noDialogDebug = false
	// Чтоб на десктопе не перезапускать приложение при завершении передачи
	noRestart = false
)

//go:embed internal/fonts
var fsFonts embed.FS

func crocDebugMode() bool {
	switch fyne.CurrentApp().Preferences().String("debug-level") {
	case "trace", "debug":
		return true
	default:
		return false
	}
}

func crocDebugLevel() string {
	return fyne.CurrentApp().Preferences().String("debug-level")
}

var exportButton *widget.Button

func setDebug() {
	debugging := crocDebugMode()
	if debugging {
		exportButton.Show()
	} else {
		exportButton.Hide()
	}
}

func settingsTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	langBinding := binding.BindPreferenceString("lang", a.Preferences())
	langSelect := widget.NewSelect([]string{"en-US", "tr-TR", "ja-JP", "zh-CN", "zh-HK", "zh-TW", "ru-RU"}, func(selection string) {
		langBinding.Set(selection)
		if langCode != selection {
			langCode = selection
			lang := language.MustParse(selection)
			langPrinter = message.NewPrinter(lang)
			refreshWindow(a, w, 2)
		}
	})
	currentLang, _ := langBinding.Get()
	langSelect.SetSelected(currentLang)

	themeBinding := binding.BindPreferenceString("theme", a.Preferences())
	themeSelect := widget.NewSelect([]string{"system", "light", "grey", "dark", "black"}, func(selection string) {
		setThemeColor(selection)
		if currentTheme, _ := themeBinding.Get(); currentTheme != selection {
			themeBinding.Set(selection)
			a.Settings().SetTheme(appTheme)
			refreshWindow(a, w, 2)
		}
	})
	currentTheme, _ := themeBinding.Get()
	themeSelect.SetSelected(currentTheme)

	// Get list of embedded fonts
	fontSelections := []string{"default"}
	fontEntries, _ := fsFonts.ReadDir("internal/fonts")
	for _, fe := range fontEntries {
		// FiraCode-Regular.ttf -> FiraCode
		if fbase, _, split := strings.Cut(fe.Name(), "-"); split {
			found := false
			for _, fs := range fontSelections {
				if fs == fbase {
					found = true
					break
				}
			}
			if !found {
				fontSelections = append(fontSelections, fbase)
			}
		}
	}

	fontBinding := binding.BindPreferenceString("font", a.Preferences())
	fontSelect := widget.NewSelect(fontSelections, func(selection string) {
		appTheme.fontName = selection
		if currentFont, _ := fontBinding.Get(); currentFont != selection {
			fontBinding.Set(selection)
			a.Settings().SetTheme(appTheme)
			refreshWindow(a, w, 2)
		}
	})
	currentFont, _ := fontBinding.Get()
	fontSelect.SetSelected(currentFont)

	curveBinding := binding.BindPreferenceString("pake-curve", a.Preferences())
	curveSelect := widget.NewSelect([]string{"siec", "p256", "p348", "p521"}, func(selection string) {
		curveBinding.Set(selection)
	})
	currentCurve, _ := curveBinding.Get()
	curveSelect.SetSelected(currentCurve)

	hashBinding := binding.BindPreferenceString("croc-hash", a.Preferences())
	hashSelect := widget.NewSelect([]string{"imohash", "md5", "xxhash", "highway"}, func(selection string) {
		hashBinding.Set(selection)
	})
	currentHash, _ := hashBinding.Get()
	hashSelect.SetSelected(currentHash)

	hideLogoBinding := binding.BindPreferenceBool("hide-logo", a.Preferences())
	toggleLogo := widget.NewButton(lp("Show / Hide"), func() {
		hideLogo, _ := hideLogoBinding.Get()
		hideLogoBinding.Set(!hideLogo)
		refreshWindow(a, w, 2)
	})

	exportButton = widget.NewButtonWithIcon(lp("Export full log"), theme.ContentCopyIcon(), func() {
		log.Tracef("Log copied to clipboard")

		s := logoutput.buf.String()
		a.Clipboard().SetContent(s)

		child := "crocdebuglog.txt"
		//	fileSave
		_ = func(destination fyne.URIWriteCloser, err error) {
			go func() {
				dialogResponse := make(chan bool, 1)

				if err != nil {
					log.Errorf("NewFileSave error: %v", err)

					fyne.Do(func() {
						a.Clipboard().SetContent(MaterialFiles)
						dialog.ShowConfirm(
							lp("Export full log")+" Download?",
							INSTALL,
							func(userConfirmed bool) {
								dialogResponse <- userConfirmed
							},
							w,
						)
					})

					var quit bool
					select {
					case userConfirmed := <-dialogResponse:
						quit = !userConfirmed
						log.Tracef("User responded to download dialog: %t", userConfirmed)
					case <-done:
						log.Warn("Download confirmation dialog canceled")
						return
					}

					if quit {
						log.Trace("User cancelled download operation")
						return
					}

					u, cl, err := ChildDownload(child)
					if err != nil {
						log.Errorf("Failed to create download location for %s: %v", child, err)
						return
					}
					defer cl()

					destination, err = storage.Writer(u)
					if err != nil {
						log.Errorf("Failed to create writer for URI %s: %v", u, err)
						return
					}
					defer destination.Close()

					if _, err := destination.Write([]byte(s)); err != nil {
						log.Errorf("Failed to write log to file: %v", err)
						return
					}

					log.Tracef("Log successfully saved to URI(%s)", u)

				} else if destination == nil {
					log.Trace("User canceled file selection")
					return
				} else {
					defer destination.Close()
					if _, err := logoutput.buf.WriteTo(destination); err != nil {
						log.Errorf("Failed to write log to selected file: %v", err)
						return
					}
					log.Trace("Log successfully saved to selected location")
				}
			}() // Конец горутины
		}

		fileSave := func(destination fyne.URIWriteCloser, err error) {
			var (
				u  fyne.URI
				cl = func() {}
			)
			if err != nil {
				log.Errorf("NewFileSave %v", err)
			} else if destination == nil {
				log.Trace("User canceled folder selection")
				return
			}

			if destination == nil {
				u, cl, err = ChildDownload(child)
				if err != nil {
					log.Errorf("append child %s to Downloads: %v", child, err)
					return
				}
				defer cl()

				destination, err = storage.Writer(u)
				if err != nil {
					log.Errorf("creating writer from URI(%s): %v", u, err)
					return
				}
			}
			defer destination.Close()

			if _, err := logoutput.buf.WriteTo(destination); err != nil {
				log.Errorf("Failed to write log to URI(%s) error: %v", u, err)
				return
			}
			log.Tracef("Log successfully saved to URI(%s)", u)

			// if _, err := destination.Write([]byte(s)); err != nil {
			// 	log.Errorf("Failed to write log to file: %v", err)
			// 	return
			// }
		}

		supported, err := IsSaveDialogSupported()
		if err != nil {
			log.Errorf("Error checking file picker support: %v", err)
			supported = false
		}
		if !supported {
			fileSave(nil, fmt.Errorf("file picker not supported"))
			log.Trace("File picker not supported. ", INSTALL)
			a.Clipboard().SetContent(MaterialFiles)
			dialog.ShowInformation(
				lp("Saved all files to")+" Download",
				INSTALL,
				w,
			)
			return
		}
		savedialog := dialog.NewFileSave(fileSave, w)
		savedialog.SetFileName(child)
		savedialog.Resize(w.Canvas().Size())
		savedialog.Show()
	})

	debugLevelBinding := binding.BindPreferenceString("debug-level", a.Preferences())
	debugCheck := widget.NewCheck(lp("Enable Debug Log"), func(debug bool) {
		if debug {
			log.SetLevel("trace")
			debugLevelBinding.Set("trace")
		} else {
			log.SetLevel("error")
			debugLevelBinding.Set("error")
		}
		setDebug()
		logoutput.buf.Reset()
	})
	debugCheck.SetChecked(crocDebugMode())

	return container.NewTabItemWithIcon(lp("Settings"), theme.SettingsIcon(), container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle(lp("Debug"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(
			debugCheck,
			layout.NewSpacer(),
			exportButton,
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Appearance"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("Language"), langSelect),
			widget.NewFormItem(lp("Theme"), themeSelect),
			widget.NewFormItem(lp("Font"), fontSelect),
			widget.NewFormItem(lp("Logo"), toggleLogo),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Relay"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("Address"), widget.NewEntryWithData(binding.BindPreferenceString("relay-address", a.Preferences()))),
			widget.NewFormItem(lp("Ports"), widget.NewEntryWithData(binding.BindPreferenceString("relay-ports", a.Preferences()))),
			widget.NewFormItem(lp("Password"), widget.NewEntryWithData(binding.BindPreferenceString("relay-password", a.Preferences()))),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Network Local"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("", widget.NewCheckWithData(lp("Disable Local"), binding.BindPreferenceBool("disable-local", a.Preferences()))),
			widget.NewFormItem("", widget.NewCheckWithData(lp("Force Local Only"), binding.BindPreferenceBool("force-local", a.Preferences()))),
			widget.NewFormItem(lp("Multicast Address"), widget.NewEntryWithData(binding.BindPreferenceString("multicast-address", a.Preferences()))),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Transfer Options"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("PAKE Curve"), curveSelect),
			widget.NewFormItem(lp("Hash Algorithm"), hashSelect),
			widget.NewFormItem("", widget.NewCheckWithData(lp("Disable Multiplexing"), binding.BindPreferenceBool("disable-multiplexing", a.Preferences()))),
			widget.NewFormItem("", widget.NewCheckWithData(lp("Disable Compression"), binding.BindPreferenceBool("disable-compression", a.Preferences()))),
			widget.NewFormItem(lp("Upload Speed Throttle"), widget.NewEntryWithData(binding.BindPreferenceString("upload-throttle", a.Preferences()))),
		),
	)))
}
