package main

import (
	"crocgui/internal/croctheme"
	"fmt"
	"image/color"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/ulikunitz/xz"
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
	ftype := "Regular"
	if style.Bold {
		ftype = "Bold"
	}

	if ttfxz, lerr := fsFonts.Open(fmt.Sprintf("internal/fonts/%s-%s.ttf.xz", c.fontName, ftype)); lerr == nil {
		xr, _ := xz.NewReader(ttfxz)
		ttf, _ := io.ReadAll(xr)
		return fyne.NewStaticResource(fmt.Sprintf("%s-%s.ttf", c.fontName, ftype), ttf)
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
