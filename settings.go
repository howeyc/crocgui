package main

import (
	"embed"
	"strings"

	log "github.com/schollz/logger"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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

var debugObjects []fyne.CanvasObject

func setDebugObjects() {
	debugging := crocDebugMode()
	for _, obj := range debugObjects {
		if debugging {
			obj.Show()
		} else {
			obj.Hide()
		}
	}
}

func settingsTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	langBinding := binding.BindPreferenceString("lang", a.Preferences())
	langSelect := widget.NewSelect([]string{"en-US", "tr-TR", "ja-JP", "zh-CN", "zh-TW"}, func(selection string) {
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
	hashSelect := widget.NewSelect([]string{"imohash", "md5", "xxhash"}, func(selection string) {
		hashBinding.Set(selection)
	})
	currentHash, _ := hashBinding.Get()
	hashSelect.SetSelected(currentHash)

	debugLevelBinding := binding.BindPreferenceString("debug-level", a.Preferences())
	debugCheck := widget.NewCheck(lp("Enable Debug Log"), func(debug bool) {
		if debug {
			log.SetLevel("trace")
			debugLevelBinding.Set("trace")
		} else {
			log.SetLevel("error")
			debugLevelBinding.Set("error")
		}
		setDebugObjects()
	})
	debugCheck.SetChecked(crocDebugMode())

	return container.NewTabItemWithIcon(lp("Settings"), theme.SettingsIcon(), container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle(lp("Appearance"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("Language"), langSelect),
			widget.NewFormItem(lp("Theme"), themeSelect),
			widget.NewFormItem(lp("Font"), fontSelect),
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
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Debug"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("", debugCheck),
		),
	)))
}
