package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/schollz/croc/v10/src/tcp"
	"github.com/schollz/croc/v10/src/utils"
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

	setupButton := widget.NewButtonWithIcon("Deep Link", theme.SettingsIcon(), func() {
		notFinish = true
		openAppSettings()
	})
	label := widget.NewLabel(lp("Click the link below to test. If a browser opens, setup is required. If the app restarts, it's OK."))
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
					time.Sleep(time.Millisecond * 7)
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
			fyne.Do(d.Hide)
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

	d := dialog.NewCustom("Deep Link", "Ok", container.NewVBox(
		label,
	), w)
	d.Resize(fyne.NewSize(300, 0))
	d.Show()
	go func() {
		timer := time.NewTimer(time.Second * 3)
		defer timer.Stop()

		select {
		case <-timer.C:
			fyne.Do(d.Hide)
		case <-done:
			return
		}
	}()
}

func setClipboard(code string, a fyne.App) (text string) {
	name := a.Preferences().String("new-relay")
	if name == "" {
		name = relayName(a)
	}
	relayAddress := a.Preferences().String("relay-address")
	if host := a.Preferences().String("host"); host != "" && host != "..." {
		// Если включен локальный посредник
		log.Debugf("host %s", host)
		relayAddress = host
		if publicIP, err := utils.PublicIP(); err == nil {
			log.Debugf("public IP %s", publicIP)
			ports := a.Preferences().String("relay-ports")
			if ports == "" {
				ports = strconv.Itoa(DEFAULT_PORT)
			} else {
				ports = strings.Split(ports, ",")[0]
			}
			address := net.JoinHostPort(publicIP, ports)
			if err := tcp.PingServer(address); err == nil {
				// Если виден снаружи
				relayAddress = publicIP
				log.Infof("croc IP %s", relayAddress)
			} else {
				// Оставляем локальный IP
				log.Debugf("could not ping: %+v", err)
			}
		}
	}
	text = toURI(
		code,
		name,
		relayAddress,
		a.Preferences().String("relay6"),
		a.Preferences().String("relay-ports"),
		a.Preferences().String("relay-password"),
		a.Preferences().String("socks5"),
		a.Preferences().String("connect"),
	)
	a.Clipboard().SetContent(text)
	return
}

func scanner() {
	// Иерархия 2026: Бренды -> Google GMS -> Сообщество -> Маркет
	links := []string{
		// 1. XIAOMI / REDMI / POCO
		"intent://#Intent;component=com.xiaomi.scanner/.app.ScanActivity;end",

		// 2. SAMSUNG (Optical Reader / Quick Scan)
		"intent://#Intent;action=com.samsung.android.app.opticalreader.SCAN;package=com.samsung.android.app.opticalreader;end",

		// 3. HUAWEI / HONOR (AI Lens)
		"intent://#Intent;action=com.huawei.scanner.action.SCAN;package=com.huawei.scanner;end",
		"intent://#Intent;action=com.huawei.hms.actions.scanservice.SCAN;package=com.huawei.scanner;end",

		// 4. GOOGLE GMS (Самый быстрый нативный сканер в Google Play Services)
		"intent://#Intent;action=com.google.android.gms.actions.SCAN_QR_CODE;package=com.google.android.gms;end",

		// 5. GOOGLE LENS (Универсальный fallback для всех Android с GMS)
		"googlelens://v1/",

		// 6. OPPO / REALME / ONEPLUS (ColorOS/Oplus)
		"intent://#Intent;component=com.oplus.scanner/.ScanActivity;end",
		"intent://#Intent;component=com.coloros.scanner/.ScanActivity;end",

		// 7. VIVO / IQOO
		"intent://#Intent;action=com.vivo.scanner.SCAN;package=com.vivo.scanner;end",

		// 8. BINARY EYE
		"intent://scan/#Intent;scheme=binaryeye;package=de.markusfisch.android.binaryeye;end",

		// 9. ZXING
		"intent://#Intent;action=com.google.zxing.client.android.SCAN;package=com.example.barcodescanner;end",

		// 10. ОБЩИЙ ИНТЕНТ
		"intent://#Intent;action=com.google.zxing.client.android.SCAN;category=android.intent.category.DEFAULT;end",
		"market://search?q=pname:de.markusfisch.android.binaryeye",
	}

	for _, s := range links {
		u, err := url.Parse(s)
		if err != nil {
			continue
		}
		if err := fyne.CurrentApp().OpenURL(u); err == nil {
			log.Debugf("find %s", s)
			return
		}
	}
}
