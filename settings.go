package main

import (
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
	default:
		// TODO: get system
		a.Settings().SetTheme(theme.LightTheme())
	}
}

func settingsTabItem(a fyne.App) *container.TabItem {
	themeBinding := binding.BindPreferenceString("theme", a.Preferences())
	themeSelect := widget.NewSelect([]string{"light", "dark"}, func(selection string) {
		setTheme(selection)
		themeBinding.Set(selection)
	})
	currentTheme, _ := themeBinding.Get()
	themeSelect.SetSelected(currentTheme)
	return container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), container.NewVBox(
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
		widget.NewLabelWithStyle("Appearance", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("Theme", themeSelect),
		),
	))
}
