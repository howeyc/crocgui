package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v8/src/croc"
	"github.com/schollz/croc/v8/src/utils"
)

func sendTabItem(a fyne.App, w fyne.Window) *container.TabItem {
	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			status.SetText(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()
	topline := widget.NewLabel("Pick a file to send")
	var currentCode string
	copyCodeButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		if currentCode != "" {
			w.Clipboard().SetContent(currentCode)
		}
	})
	copyCodeButton.Hide()
	return container.NewTabItemWithIcon("Send", theme.MailSendIcon(),
		container.NewVBox(
			topline,
			widget.NewButtonWithIcon("File", theme.FileIcon(), func() {
				dialog.ShowFileOpen(func(f fyne.URIReadCloser, e error) {
					if e != nil {
						return
					}
					randomName := utils.GetRandomName()
					sender, err := croc.New(croc.Options{
						IsSender:      true,
						SharedSecret:  randomName,
						Debug:         false,
						RelayAddress:  a.Preferences().String("relay-address"),
						RelayPorts:    strings.Split(a.Preferences().String("relay-ports"), ","),
						RelayPassword: a.Preferences().String("relay-password"),
						Stdout:        false,
						NoPrompt:      true,
						DisableLocal:  true,
					})
					var filename string
					if err != nil {
						log.Println(err)
					} else if f != nil {
						fpath := fixpath(f.URI().Path())

						fi, sterr := os.Stat(fpath)
						if sterr != nil {
							status.SetText(fmt.Sprintf("Stat error: %s - %s", fpath, sterr.Error()))
							return
						}
						status.SetText("Receive Code: " + randomName)
						currentCode = randomName
						copyCodeButton.Show()
						filename = filepath.Base(fpath)
						topline.SetText(fmt.Sprintf("Sending file: %s", filename))
						totalsize := fi.Size()
						prog.Max = float64(totalsize)
						prog.Show()
						donechan := make(chan bool)
						go func() {
							ticker := time.NewTicker(time.Millisecond * 100)
							for {
								select {
								case <-ticker.C:
									prog.SetValue(float64(sender.TotalSent))
								case <-donechan:
									ticker.Stop()
									return
								}
							}
						}()
						go func() {
							serr := sender.Send(croc.TransferOptions{
								PathToFiles: []string{fpath},
							})
							donechan <- true
							prog.Hide()
							prog.SetValue(0)
							topline.SetText("Pick a file to send")
							if serr != nil {
								log.Println("Send failed:", serr)
							} else {
								status.SetText(fmt.Sprintf("Sent file %s", filename))
							}
							currentCode = ""
							copyCodeButton.Hide()
						}()
					}
				}, w)
			}),
			prog,
			container.NewHBox(status, copyCodeButton),
		))
}
