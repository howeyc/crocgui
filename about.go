package main

import (
	"bufio"
	"bytes"
	"embed"
	_ "embed"
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

func aboutTabItem() *container.TabItem {
	longdescbytes, _ := metadata.ReadFile(fmt.Sprintf("metadata/%s/full_description.txt", langCode))
	longdesc := string(longdescbytes)
	longdesc = strings.ReplaceAll(longdesc, "<b>", "")
	longdesc = strings.ReplaceAll(longdesc, "</b>", "")
	aboutInfo := widget.NewLabel(longdesc)
	aboutInfo.Wrapping = fyne.TextWrapWord

	acLicense := widget.NewAccordion()

	licenseReader := bytes.NewBufferString(crocguiLicense + thirdPartyLicenses)
	currentLicense := ""
	currentLibrary := "croc"
	scanner := bufio.NewScanner(licenseReader)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "-----") {
			acLicense.Append(widget.NewAccordionItem(currentLibrary, widget.NewLabel(currentLicense)))
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
				acLicense.Append(widget.NewAccordionItem(fmt.Sprintf("Font: %s", fbase), widget.NewLabel(strLicense)))
			}
		}
	}

	licenseToggle := widget.NewButton(lp("License Info"), func() {
		w := fyne.CurrentApp().NewWindow(lp("License Info"))
		w.SetContent(container.NewScroll(acLicense))
		w.Resize(fyne.NewSize(450, 800))
		w.Show()
	})
	return container.NewTabItemWithIcon(lp("About"), theme.InfoIcon(),
		container.NewVScroll(container.NewVBox(aboutInfo, licenseToggle)),
	)
}
