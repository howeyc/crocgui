// tree.go
package main

import (
	"net"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xw "fyne.io/x/fyne/widget"
	log "github.com/schollz/logger"
)

func createTreeButton(
	parent fyne.Window,
	scroller *container.Scroll,
	boxholder *fyne.Container,
	app fyne.App,
) (*widget.Button, func()) {

	server := GetWebDAVServer()
	var treeButton *widget.Button
	treeButton = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		u := storage.NewFileURI(tempDir)

		if !(isMobile || asMobile) {
			if err := OpenURL(u.String()); err == nil {
				return
			} else {
				log.Errorf("OpenURL: %v", err)
			}
		}

		if scroller.Content == boxholder {
			// Переключаемся на tree view
			treeButton.SetIcon(theme.VisibilityOffIcon())

			ft := xw.NewFileTree(u)
			ft.OpenAllBranches()

			updateLink := func() {}
			prev := hostSelectOptions(LOCAL)[0]

			// Получаем настройки из предпочтений
			prefHost := app.Preferences().StringWithFallback("webdav-host", prev)
			prefPort := app.Preferences().StringWithFallback("webdav-port", "8080")

			hostSelect := NewSelect(hostSelectOptions(LOCAL), func(next string) {
				if next != prev {
					prev = next
					app.Preferences().SetString("webdav-host", next)
					updateLink()
				}
			})

			link := widget.NewHyperlink("", nil)
			port := widget.NewEntry()

			port.OnChanged = func(s string) {
				app.Preferences().SetString("webdav-port", s)
			}

			updateLink = func() {
				addr := net.JoinHostPort(hostSelect.Selected, port.Text)

				// Используем умный запуск/обновление
				err := server.StartOrUpdate(addr, tempDir)
				if err != nil {
					log.Errorf("Failed to start WebDAV server: %v", err)
					link.SetText("Ошибка: " + err.Error())
					link.URL = nil
					return
				}

				// Получаем текущий URL
				url := server.GetURL()
				if url == "" {
					link.SetText("Сервер не активен")
					link.URL = nil
					return
				}

				link.SetText(url)
				link.SetURLFromString(url)

				// Логируем статус
				currAddr, currDir, active, refCount := server.GetStatus()
				log.Debugf("WebDAV: addr=%s, dir=%s, active=%v, users=%d",
					currAddr, currDir, active, refCount)
			}

			port.OnSubmitted = func(s string) {
				updateLink()
			}

			// Устанавливаем значения из настроек или текущего сервера
			currAddr, _, active, _ := server.GetStatus()
			if active && currAddr != "" {
				// Используем текущие параметры сервера
				if host, portStr, err := net.SplitHostPort(currAddr); err == nil {
					hostSelect.SetSelected(host)
					port.SetText(portStr)

					// Обновляем настройки если они изменились
					if host != prefHost {
						app.Preferences().SetString("webdav-host", host)
					}
					if portStr != prefPort {
						app.Preferences().SetString("webdav-port", portStr)
					}
				}
			} else {
				// Используем сохраненные настройки
				hostSelect.SetSelected(prefHost)
				port.SetText(prefPort)
			}

			updateLink()

			top := container.NewBorder(
				nil,
				nil,
				container.NewGridWrap(widget.NewLabel("\t\t\t").MinSize(), hostSelect),
				link,
				container.NewGridWrap(widget.NewLabel("\t").MinSize(), port),
			)
			scroller.Content = container.NewBorder(top, nil, nil, nil, ft)
			scroller.Refresh()
			return
		}

		// Переключаемся обратно на список файлов
		treeButton.SetIcon(theme.VisibilityIcon())
		scroller.Content = boxholder
		scroller.Refresh()

		// Уменьшаем счетчик использования
		if err := server.Stop(); err != nil {
			log.Errorf("Failed to decrease ref count: %v", err)
		}
	})

	// Функция для полной остановки при закрытии приложения
	treeOff := func() {
		if err := GetWebDAVServer().StopNow(); err != nil {
			log.Errorf("Force stop WebDAV failed: %v", err)
		}
	}

	return treeButton, treeOff
}
