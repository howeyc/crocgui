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
		widget.NewForm(
			widget.NewFormItem("Relay Address", widget.NewEntryWithData(binding.BindPreferenceString("relay-address", a.Preferences()))),
			widget.NewFormItem("Relay Password", widget.NewEntryWithData(binding.BindPreferenceString("relay-password", a.Preferences()))),
			widget.NewFormItem("Relay Ports", widget.NewEntryWithData(binding.BindPreferenceString("relay-ports", a.Preferences()))),
		),
	))
}
