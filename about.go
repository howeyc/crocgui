package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed metadata/en-US/full_description.txt
var longdesc string

//go:embed LICENSE
var crocguiLicense string

//go:embed third-party-licenses.txt
var thirdPartyLicenses string

func aboutTabItem() *container.TabItem {
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

	licenseToggle := widget.NewButton("License Info", func() {
		w := fyne.CurrentApp().NewWindow("licenses")
		w.SetContent(container.NewScroll(acLicense))
		w.Resize(fyne.NewSize(450, 800))
		w.Show()
	})
	return container.NewTabItemWithIcon("About", theme.InfoIcon(),
		container.NewVScroll(container.NewVBox(aboutInfo, licenseToggle)),
	)
}
