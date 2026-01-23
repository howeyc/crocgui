package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
	"github.com/skip2/go-qrcode"
)

func fromURI(u string) (st, ne, as, a6, ps, pd, s5, ct string, err error) {
	if len(u) <= len(IO) || !strings.HasPrefix(u, IO) {
		err = fmt.Errorf("not IO")
		return
	}

	fragment := strings.TrimPrefix(u, IO)
	log.Debug(fragment)
	decoded, err := base64.RawURLEncoding.DecodeString(fragment)
	log.Debug(string(decoded))
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(fragment)
		log.Error(string(decoded))
		if err != nil {
			return
		}
	}

	str := strings.TrimRight(string(decoded), "\n")
	//log.Debug(str)
	ss := strings.Split(str, "\n")
	for i, s := range ss {
		switch i {
		case 0:
			st = s
		case 1:
			ne = s
		case 2:
			as = s
		case 3:
			a6 = s
		case 4:
			ps = s
		case 5:
			pd = s
		case 6:
			s5 = s
		case 7:
			ct = s
		default:
			return
		}
	}
	log.Debugf("%v", ss)

	return
}

// st, ne, as, a6, ps, pd, s5, ct
func toURI(ss ...string) (u string) {
	for i, s := range ss {
		if i > 7 {
			break
		}
		s = strings.ReplaceAll(s, "\n", "")
		u += strings.TrimSpace(s) + "\n"
	}
	u = strings.TrimRight(u, "\n")
	log.Debug(IO + u)
	return IO + base64.RawURLEncoding.EncodeToString([]byte(u))
}
func showQR(a fyne.App, w fyne.Window, text string) {
	if text == "" {
		text = a.Clipboard().Content()
		if text == "" {
			log.Error("empty clipboard")
			return
		}
	}

	var pngData []byte
	pngData, err := qrcode.Encode(text, qrcode.High, 256)
	if err != nil {
		log.Error(err)
		return
	}

	img := canvas.NewImageFromReader(bytes.NewReader(pngData), "qr.png")
	img.SetMinSize(fyne.NewSize(256, 256))
	img.FillMode = canvas.ImageFillContain

	setupButton := widget.NewButtonWithIcon(lp("Set up Deep Link handling"), theme.SettingsIcon(), func() {
		notFinish = true
		openAppSettings()
	})
	label := widget.NewLabel(lp("Click the Deep Link to test. If a browser opens, setup is required. If the app restarts, it's OK."))
	label.Wrapping = fyne.TextWrapWord
	if !isAndroid {
		setupButton.Hide()
		label.Hide()
	}

	link := widget.NewHyperlink(text, nil)
	link.SetURLFromString(text)
	link.Wrapping = fyne.TextWrapBreak

	content := container.NewVBox(
		setupButton,
		label,
		link,
		img,
	)

	d := dialog.NewCustom("Deep Link", "Ok", content, w)

	link.OnTapped = func() {
		d.Hide()
		u, err := url.Parse(text)
		if err == nil {
			if isAndroid {
				// notFinish = true
				go func() {
					time.Sleep(time.Millisecond * 33)
					os.Exit(0)
				}()
			}
			a.OpenURL(u)
		}
	}

	d.Resize(fyne.NewSize(300, 0))
	d.Show()
	go func() {
		timer := time.NewTimer(time.Second * 7)
		defer timer.Stop()

		select {
		case <-timer.C:
			d.Hide()
		case <-done:
			return
		}
	}()

}

func fromClipboard(a fyne.App, w fyne.Window, st, ne, as, a6, ps, pd, s5, ct string) {
	info := fmt.Sprintf(
		"code:\t\t%s\n%s:\t\t%s\nrelay:\t\t%s\nrelay6:\t\t%s\nports:\t\t%s\npass:\t\t%s\nsocks5:\t\t%s\nconnect:\t\t%s",
		st, lp("Name"), ne, as, a6, ps, pd, s5, ct,
	)
	label := widget.NewLabel(info)
	label.Wrapping = fyne.TextWrapWord

	d := dialog.NewCustom("Deep Link", "Ok", label, w)
	d.Resize(fyne.NewSize(300, 0))
	d.Show()
	go func() {
		timer := time.NewTimer(time.Second * 3)
		defer timer.Stop()

		select {
		case <-timer.C:
			d.Hide()
		case <-done:
			return
		}
	}()
}
