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
	"github.com/BurntSushi/toml"
	log "github.com/schollz/logger"
)

//go:embed metadata
var metadata embed.FS

//go:embed LICENSE
var crocguiLicense string

//go:embed third-party-licenses.txt
var thirdPartyLicenses string

//go:embed FyneApp.toml
var fyneApp string

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
	currentLibrary := CROC
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

	crocHyperlink := widget.NewHyperlink(fmt.Sprintf("%s/%s/%s", GH, SCHOLLZ, CROC), nil)
	crocHyperlink.SetURLFromString(fmt.Sprintf("%s://%s/%s/%s/releases/latest", HTTPS, GH, SCHOLLZ, CROC))

	fromHyperlink := widget.NewHyperlink(fmt.Sprintf("%s/%s/%s v%s_%d", GH, FORKfrom, "crocgui", FORKfromVersion, FORKfromBuild), nil)
	fromHyperlink.SetURLFromString(fmt.Sprintf("%s://%s/%s/%s/releases/tag/v%s", HTTPS, GH, FORKfrom, "crocgui", FORKfromVersion))

	ve, bu, errVb := VersionBuild(a, fyneApp)
	oldHyperlink := widget.NewHyperlink(fmt.Sprintf("%s/%s/%s v%s_%d", GH, FORKto, CG, ve, bu), nil)
	oldHyperlink.SetURLFromString(fmt.Sprintf("%s://%s/%s/%s/releases/tag/v%s", HTTPS, GH, FORKto, CG, ve))
	oldHyperlink.Hidden = errVb != nil

	newHyperlink := widget.NewHyperlink("", nil)
	newHyperlink.Hidden = true
	appInfo := widget.NewButtonWithIcon(lp("App info"), theme.InfoIcon(), func() {
		idActions(ID, APPLICATION_DETAILS_SETTINGS)
	})
	appInfo.Hidden = !isAndroid

	ti := container.NewTabItemWithIcon("", theme.InfoIcon(), //lp("About")
		container.NewVScroll(container.NewVBox(
			appInfo,
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
	OnSelectedTab[ABOUTi] = func() {
		go func() {
			latestVersion, err := Latest(FORKto, CG)
			currentVersion := fmt.Sprintf("v%s", ve)
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

// FyneApp describes the top level metadata for building a fyne application
type FyneApp struct {
	Website     string `toml:",omitempty"`
	Description string `toml:",omitempty"`
	Details     AppDetails
	Development map[string]string `toml:",omitempty"`
	Release     map[string]string `toml:",omitempty"`
	Source      *AppSource        `toml:",omitempty"`
	CanOpen     *CanOpen          `toml:",omitempty"`
	LinuxAndBSD *LinuxAndBSD      `toml:",omitempty"`
	Languages   []string          `toml:",omitempty"`
	Migrations  map[string]bool   `toml:",omitempty"`
}

// AppDetails describes the build information, this group may be OS or arch specific
type AppDetails struct {
	Icon     string `toml:",omitempty"`
	Name, ID string `toml:",omitempty"`
	Version  string `toml:",omitempty"`
	Build    int    `toml:",omitempty"`
}

type AppSource struct {
	Repo, Dir string `toml:",omitempty"`
}

// LinuxAndBSD describes specific metadata for desktop files on Linux and BSD.
type LinuxAndBSD struct {
	GenericName string   `toml:",omitempty"`
	Categories  []string `toml:",omitempty"`
	Comment     string   `toml:",omitempty"`
	Keywords    []string `toml:",omitempty"`
	ExecParams  string   `toml:",omitempty"`
}

// CanOpen represents a selection of file types (mime etc) that this application can open.
type CanOpen struct {
	MimeTypes string `toml:",omitempty"`
}

// VersionBuild возвращает строку с версией и билдом
// Приоритет: метаданные приложения > переданный FyneApp.toml (только при Build <= 1)
func VersionBuild(a fyne.App, fallback string) (string, int, error) {
	md := a.Metadata()

	version := md.Version
	build := md.Build

	if build > 1 {
		return version, build, nil
	}

	if fallback == "" {
		return "", 0, fmt.Errorf("no fallback")
	}

	var data FyneApp
	if _, err := toml.Decode(fallback, &data); err != nil {
		return "", 0, err
	}

	version = data.Details.Version
	if version == "" {
		return "", 0, fmt.Errorf("no version")
	}

	build = data.Details.Build

	return version, build, nil
}
