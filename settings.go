package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func settingsTabItem(a fyne.App) *container.TabItem {
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
	))
}
