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
	log "github.com/schollz/logger"
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
	md := a.Metadata()
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

	crocHyperlink := widget.NewHyperlink(fmt.Sprintf("%s/%s/%s", GH, "schollz", "croc"), nil)
	crocHyperlink.SetURLFromString(fmt.Sprintf("https://%s/%s/%s/releases/latest", GH, "schollz", "croc"))
	fromHyperlink := widget.NewHyperlink(fmt.Sprintf("%s/%s/%s v%s.%d", GH, FORKfrom, CG, FORKfromVersion, FORKfromBuild), nil)
	fromHyperlink.SetURLFromString(fmt.Sprintf("https://%s/%s/%s/releases/tag/v%s", GH, FORKfrom, CG, FORKfromVersion))
	oldHyperlink := widget.NewHyperlink(fmt.Sprintf("%s/%s/%s v%s.%d", GH, FORKto, CG, md.Version, md.Build), nil)
	oldHyperlink.SetURLFromString(fmt.Sprintf("https://%s/%s/%s/releases/tag/v%s", GH, FORKto, CG, md.Version))
	newHyperlink := widget.NewHyperlink("", nil)
	newHyperlink.Hidden = true

	ti := container.NewTabItemWithIcon(ZeroWidthJoiner, theme.InfoIcon(), //lp("About")
		container.NewVScroll(container.NewVBox(
			container.New(&tightVBoxLayout{},
				crocHyperlink,
				fromHyperlink,
				oldHyperlink,
				newHyperlink,
			),
			aboutInfo,
			licenseToggle,
		)),
	)
	OnSelectedReload[4] = func() {
		go func() {
			latestVersion, err := Latest(FORKto, CG)
			currentVersion := fmt.Sprintf("v%s", md.Version)
			log.Debugf("%s %s %v", currentVersion, latestVersion, err)

			if err != nil || newHyperlink == nil || latestVersion == currentVersion {
				return
			}
			url := fmt.Sprintf("%s/%s/%s/releases/tag/%s",
				GH, FORKto, CG, latestVersion)

			fyne.Do(func() {
				newHyperlink.SetText(url)
				newHyperlink.SetURLFromString("https://" + url)
				newHyperlink.Hidden = false

				ti.Content.Refresh()
			})
		}()
	}
	return ti
}

type tightVBoxLayout struct{}

const tight = 0.6

func (t *tightVBoxLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	yPos := float32(0)
	for _, obj := range objects {
		if obj.Visible() {
			obj.Resize(fyne.NewSize(size.Width, obj.MinSize().Height))
			obj.Move(fyne.NewPos(0, yPos))
			yPos += obj.MinSize().Height * tight
		}
	}
}

func (t *tightVBoxLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	height := float32(0)
	width := float32(0)
	for _, obj := range objects {
		if obj.Visible() {
			height += obj.MinSize().Height * tight
			if obj.MinSize().Width > width {
				width = obj.MinSize().Width
			}
		}
	}
	return fyne.NewSize(width, height)
}
