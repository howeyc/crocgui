package main

import (
	"crocgui/internal/croctheme"

	log "github.com/schollz/logger"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func setTheme(themeName string) {
	a := fyne.CurrentApp()
	switch themeName {
	case "light":
		a.Settings().SetTheme(theme.LightTheme())
	case "dark":
		a.Settings().SetTheme(theme.DarkTheme())
	case "black":
		a.Settings().SetTheme(croctheme.BlackTheme())
	default:
		// TODO: get system
		a.Settings().SetTheme(theme.LightTheme())
	}
}

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

func settingsTabItem(a fyne.App) *container.TabItem {
	themeBinding := binding.BindPreferenceString("theme", a.Preferences())
	themeSelect := widget.NewSelect([]string{"light", "dark", "black"}, func(selection string) {
		setTheme(selection)
		themeBinding.Set(selection)
	})
	currentTheme, _ := themeBinding.Get()
	themeSelect.SetSelected(currentTheme)

	debugLevelBinding := binding.BindPreferenceString("debug-level", a.Preferences())
	debugCheck := widget.NewCheck("Enable Debug Log", func(debug bool) {
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

	return container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("Appearance", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("Theme", themeSelect),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Relay", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("Address", widget.NewEntryWithData(binding.BindPreferenceString("relay-address", a.Preferences()))),
			widget.NewFormItem("Ports", widget.NewEntryWithData(binding.BindPreferenceString("relay-ports", a.Preferences()))),
			widget.NewFormItem("Password", widget.NewEntryWithData(binding.BindPreferenceString("relay-password", a.Preferences()))),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Network Local", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("", widget.NewCheckWithData("Disable Local", binding.BindPreferenceBool("disable-local", a.Preferences()))),
			widget.NewFormItem("", widget.NewCheckWithData("Force Local Only", binding.BindPreferenceBool("force-local", a.Preferences()))),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Transfer Options", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("", widget.NewCheckWithData("Disable Multiplexing", binding.BindPreferenceBool("disable-multiplexing", a.Preferences()))),
			widget.NewFormItem("", widget.NewCheckWithData("Disable Compression", binding.BindPreferenceBool("disable-compression", a.Preferences()))),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Debug", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("", debugCheck),
		),
	)))
}
