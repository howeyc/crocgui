package main

import (
	_ "embed"
	"net/url"
	"sort"
	"strings"

	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v8/src/croc"
	"github.com/schollz/croc/v8/src/utils"
)

func parseURL(s string) *url.URL {
	link, _ := url.Parse(s)
	return link
}

func aboutTabItem() *container.TabItem {
	return container.NewTabItemWithIcon("About", theme.InfoIcon(), container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("croc GUI", widget.NewHyperlink("v1.0.0", parseURL("https://github.com/howeyc/crocgui"))),
			widget.NewFormItem("croc", widget.NewHyperlink("v8.6.7", parseURL("https://github.com/schollz/croc"))),
		),
	))
}

func sendTabItem(w fyne.Window) *container.TabItem {
	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			status.SetText(fmt.Sprint(r))
		}
	}()
	prog := widget.NewProgressBar()
	prog.Hide()
	topline := widget.NewLabel("Pick a file to send")
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
						RelayAddress:  "croc.schollz.com:9009",
						RelayPorts:    []string{"9009", "9010", "9011", "9012", "9013"},
						RelayPassword: "pass123",
						Stdout:        false,
						NoPrompt:      true,
						DisableLocal:  true,
					})
					var filename string
					if err != nil {
						log.Println(err)
					} else if f != nil {
						status.SetText("Receive Code: " + randomName)
						fpath := fixpath(f.URI().Path())

						fi, sterr := os.Stat(fpath)
						if sterr != nil {
							status.SetText(fmt.Sprintf("Stat error: %s - %s", fpath, sterr.Error()))
							return
						}
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
						}()
					}
				}, w)
			}),
			prog,
			status,
		))
}

func recvTabItem() *container.TabItem {
	status := widget.NewLabel("")
	defer func() {
		if r := recover(); r != nil {
			status.SetText(fmt.Sprint(r))
		}
	}()

	prog := widget.NewProgressBar()
	prog.Hide()
	recvEntry := widget.NewEntry()
	topline := widget.NewLabel("Enter code to download")
	return container.NewTabItemWithIcon("Receive", theme.DownloadIcon(),
		container.NewVBox(
			topline,
			widget.NewForm(&widget.FormItem{Text: "Receive Code", Widget: recvEntry}),
			widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
				receiver, err := croc.New(croc.Options{
					IsSender:      false,
					SharedSecret:  recvEntry.Text,
					Debug:         false,
					RelayAddress:  "croc.schollz.com:9009",
					RelayPassword: "pass123",
					Stdout:        false,
					NoPrompt:      true,
					DisableLocal:  true,
				})
				if err != nil {
					log.Println("Receive setup error:", err)
				}
				prog.Show()
				donechan := make(chan bool)
				var filename string
				receivednames := make(map[string]int)
				go func() {
					ticker := time.NewTicker(time.Millisecond * 100)
					for {
						select {
						case <-ticker.C:
							if receiver.Step2FileInfoTransfered {
								cnum := receiver.FilesToTransferCurrentNum
								fi := receiver.FilesToTransfer[cnum]
								filename = filepath.Base(fi.Name)
								receivednames[filename] = cnum
								topline.SetText(fmt.Sprintf("Receiving file: %s (%d/%d)", filename, cnum+1, len(receiver.FilesToTransfer)))
								prog.Max = float64(fi.Size)
								prog.SetValue(float64(receiver.TotalSent))
							}
						case <-donechan:
							ticker.Stop()
							return
						}
					}
				}()
				cderr := os.Chdir(DEFAULT_DOWNLOAD_DIR)
				if cderr != nil {
					log.Println("Unable to change to download dir")
				}
				status.SetText("")
				rerr := receiver.Receive()
				donechan <- true
				prog.Hide()
				prog.SetValue(0)
				topline.SetText("Enter code to download")
				if rerr != nil {
					status.Text = "Receive failed: " + rerr.Error()
				} else {
					filesReceived := make([]string, len(receivednames))
					var i int
					for f := range receivednames {
						filesReceived[i] = f
						i++
					}
					sort.Slice(filesReceived, func(i, j int) bool {
						return receivednames[filesReceived[i]] < receivednames[filesReceived[j]]
					})

					plural := ""
					if len(filesReceived) > 1 {
						plural = "s"
					}
					status.Text = fmt.Sprintf("Received file%s %s", plural, strings.Join(filesReceived, ","))
				}
			}),
			prog,
			status,
		))

}

//go:embed text-logo.png
var textlogobytes []byte

func main() {
	a := app.NewWithID("com.github.howeyc.crocgui")
	w := a.NewWindow("croc")

	textlogores := fyne.NewStaticResource("text-logo", textlogobytes)
	textlogo := canvas.NewImageFromResource(textlogores)
	textlogo.SetMinSize(fyne.NewSize(205, 100))
	top := container.NewHBox(layout.NewSpacer(), textlogo, layout.NewSpacer())
	w.SetContent(container.NewBorder(top, nil, nil, nil, container.NewAppTabs(sendTabItem(w), recvTabItem(), aboutTabItem())))
	w.Resize(fyne.NewSize(800, 600))
	w.ShowAndRun()
}
