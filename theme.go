package main

import (
	"crocgui/internal/croctheme"
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type crocTheme struct {
	color fyne.Theme
	icon  fyne.Theme

	fontName string

	size fyne.Theme
}

var _ fyne.Theme = (*crocTheme)(nil)

var appTheme = crocTheme{}

func (c crocTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return c.color.Color(name, variant)
}

func (c crocTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return c.icon.Icon(name)
}

func (c crocTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Bold {
		if ttf, lerr := fsFonts.ReadFile(fmt.Sprintf("internal/fonts/%s-Bold.ttf", c.fontName)); lerr == nil {
			return fyne.NewStaticResource(fmt.Sprintf("%s-Bold.ttf", c.fontName), ttf)
		}
	}
	if ttf, lerr := fsFonts.ReadFile(fmt.Sprintf("internal/fonts/%s-Regular.ttf", c.fontName)); lerr == nil {
		return fyne.NewStaticResource(fmt.Sprintf("%s-Regular.ttf", c.fontName), ttf)
	}

	return theme.DefaultTheme().Font(style)
}

func (c crocTheme) Size(name fyne.ThemeSizeName) float32 {
	return c.size.Size(name)
}

func setThemeColor(themeName string) {
	switch themeName {
	case "light":
		appTheme.color = theme.LightTheme()
	case "grey":
		appTheme.color = croctheme.GreyTheme()
	case "dark":
		appTheme.color = theme.DarkTheme()
	case "black":
		appTheme.color = croctheme.BlackTheme()
	case "system":
		appTheme.color = theme.DefaultTheme()
	}
}
