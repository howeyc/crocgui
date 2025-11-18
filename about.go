package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed metadata
var metadata embed.FS

//go:embed LICENSE
var crocguiLicense string

//go:embed third-party-licenses.txt
var thirdPartyLicenses string

func aboutTabItem(a fyne.App, _ fyne.Window) *container.TabItem {
	longdescbytes, _ := metadata.ReadFile(fmt.Sprintf("metadata/%s/full_description.txt", langCode))
	longdesc := string(longdescbytes)
	longdesc = strings.ReplaceAll(longdesc, "<b>", "")
	longdesc = strings.ReplaceAll(longdesc, "</b>", "")
	aboutInfo := widget.NewLabel(longdesc)
	aboutInfo.Wrapping = fyne.TextWrapWord

	// acLicense := widget.NewAccordion()
	var ais []*widget.AccordionItem

	licenseReader := bytes.NewBufferString(crocguiLicense + thirdPartyLicenses)
	currentLicense := ""
	currentLibrary := "croc"
	scanner := bufio.NewScanner(licenseReader)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "-----") {
			ais = append(ais, widget.NewAccordionItem(currentLibrary, widget.NewLabel(currentLicense)))
			currentLicense = ""
			scanner.Scan()
			scanner.Scan()
			currentLibrary = scanner.Text()
			scanner.Scan()
			continue
		}
		currentLicense += fmt.Sprintln(line)
	}

	// Add font licenses
	fontEntries, _ := fsFonts.ReadDir("internal/fonts")
	for _, fe := range fontEntries {
		if fbase, remain, split := strings.Cut(fe.Name(), "-"); split && remain == "OFL.txt" {
			bfontLicense, rerr := fsFonts.ReadFile(fmt.Sprintf("internal/fonts/%s", fe.Name()))
			if rerr == nil {
				strLicense := string(bfontLicense)
				// acLicense.Append(widget.NewAccordionItem(fmt.Sprintf("Font: %s", fbase), widget.NewLabel(strLicense)))
				ais = append(ais, widget.NewAccordionItem(fmt.Sprintf("Font: %s", fbase), widget.NewLabel(strLicense)))
			}
		}
	}

	licenseToggle := widget.NewButton(lp("License Info"), func() {
		w := a.NewWindow(lp("License Info"))

		acLicense := widget.NewAccordion(ais...)

		w.SetContent(container.NewScroll(acLicense))
		w.Resize(size)
		w.Show()
	})
	return container.NewTabItemWithIcon(ZeroWidthJoiner, theme.InfoIcon(), //lp("About")
		container.NewVScroll(container.NewVBox(aboutInfo, licenseToggle)),
	)
}
