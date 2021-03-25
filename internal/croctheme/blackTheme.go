package croctheme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

func BlackTheme() fyne.Theme {
	return &blackTheme{}
}

type blackTheme struct{}

var _ fyne.Theme = (*blackTheme)(nil)

func (b blackTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		return color.Black
	}
	if name == theme.ColorNameShadow {
		return color.White
	}

	return theme.DarkTheme().Color(name, theme.VariantDark)
}

func (b blackTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DarkTheme().Icon(name)
}

func (b blackTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DarkTheme().Font(style)
}

func (bm blackTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DarkTheme().Size(name)
}
