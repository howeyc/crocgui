package croctheme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// GreyTheme emulates dark theme from fyne v2.2.4
func GreyTheme() fyne.Theme {
	theme := &greyTheme{}
	return theme
}

var (
	darkPalette = map[fyne.ThemeColorName]color.Color{
		theme.ColorNameBackground:        color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff},
		theme.ColorNameDisabled:          color.NRGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xff},
		theme.ColorNameButton:            color.NRGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xff},
		theme.ColorNameDisabledButton:    color.NRGBA{R: 0x26, G: 0x26, B: 0x26, A: 0xff},
		theme.ColorNameOverlayBackground: color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff},
		theme.ColorNameMenuBackground:    color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff},
		theme.ColorNameForeground:        color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		theme.ColorNameHover:             color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x0f},
		theme.ColorNameInputBackground:   color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x19},
		theme.ColorNamePlaceHolder:       color.NRGBA{R: 0xb2, G: 0xb2, B: 0xb2, A: 0xff},
		theme.ColorNamePressed:           color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x66},
		theme.ColorNameSeparator:         color.NRGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xff},
		theme.ColorNameScrollBar:         color.NRGBA{A: 0x99},
		theme.ColorNameShadow:            color.NRGBA{A: 0x66},
	}
)

type greyTheme struct {
}

var _ fyne.Theme = (*greyTheme)(nil)

func (t *greyTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	v = theme.VariantDark
	colors := darkPalette

	if c, ok := colors[n]; ok {
		return c
	}

	return theme.DefaultTheme().Color(n, v)
}

func (t *greyTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *greyTheme) Size(s fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(s)
}

func (t *greyTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
