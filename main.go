package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v8/src/croc"
	"github.com/schollz/croc/v8/src/utils"
)

func sendTabItem(w fyne.Window) *container.TabItem {
	status := widget.NewLabel("")
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
						RelayAddress:  "10.0.1.1:9009",
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
						fi, _ := os.Stat(f.URI().Path())
						filename = filepath.Base(fi.Name())
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
						serr := sender.Send(croc.TransferOptions{
							PathToFiles: []string{f.URI().Path()},
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
					}
				}, w)
			}),
			prog,
			status,
		))
}

func recvTabItem() *container.TabItem {
	status := widget.NewLabel("")
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
					RelayAddress:  "10.0.1.1:9009",
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
				go func() {
					ticker := time.NewTicker(time.Millisecond * 100)
					for {
						gotInfo := false
						select {
						case <-ticker.C:
							if !gotInfo && receiver.Step2FileInfoTransfered {
								gotInfo = true
								fi := receiver.FilesToTransfer[0]
								filename = filepath.Base(fi.Name)
								topline.SetText(fmt.Sprintf("Receiving file: %s", filename))
								prog.Max = float64(fi.Size)
							}
							prog.SetValue(float64(receiver.TotalSent))
						case <-donechan:
							ticker.Stop()
							return
						}
					}
				}()
				rerr := receiver.Receive()
				donechan <- true
				prog.Hide()
				prog.SetValue(0)
				topline.SetText("Enter code to download")
				if rerr != nil {
					status.Text = "Receive failed: " + rerr.Error()
				} else {
					status.Text = fmt.Sprintf("Received file %s", filename)
				}
			}),
			prog,
			status,
		))

}

func main() {
	a := app.NewWithID("com.github.howeyc.crocgui")
	w := a.NewWindow("croc")

	w.SetContent(container.NewAppTabs(sendTabItem(w), recvTabItem()))
	w.ShowAndRun()
}
