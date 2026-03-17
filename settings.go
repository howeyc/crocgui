// settings.go
package main

import (
	"context"
	"embed"
	"encoding/json"
	"net"
	"os"
	"reflect"
	"strings"

	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/utils"
	log "github.com/schollz/logger"
	"github.com/schollz/pake/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed internal/fonts
var fsFonts embed.FS

// Структура, отражающая ключи preferences и типы данных (string, bool).
// Поле json указывает ключ preference.
type GuiPrefsData struct {
	RelayAddress  string `json:"relay-address"`
	RelayAddress6 string `json:"relay6"`
	RelayPorts    string `json:"relay-ports"` // Сохраняем как строку, объединенную через запятую
	RelayPassword string `json:"relay-password"`
	DisableLocal  bool   `json:"disable-local"`
	OnlyLocal     bool   `json:"force-local"`
	Curve         string `json:"pake-curve"`
	HashAlgorithm string `json:"croc-hash"`
	GitIgnore     bool   `json:"git"`
	Overwrite     bool   `json:"overwrite"`
}

var accordion *widget.Accordion

func settingsTabItem(a fyne.App, w fyne.Window) (ti *container.TabItem) {
	langBinding := binding.BindPreferenceString("lang", a.Preferences())
	langSelect := widget.NewSelect([]string{"en-US", "tr-TR", "ja-JP", "zh-CN", "ru-RU"}, func(selection string) {
		langBinding.Set(selection)
		if langCode != selection {
			langCode = selection
			lang := language.MustParse(selection)
			langPrinter = message.NewPrinter(lang)
			refreshWindow(a, w)
		}
	})
	currentLang, _ := langBinding.Get()
	langSelect.SetSelected(currentLang)

	themeBinding := binding.BindPreferenceString("theme", a.Preferences())
	themeSelect := widget.NewSelect([]string{"system", "light", "grey", "dark", "black"}, func(selection string) {
		setThemeColor(selection)
		if currentTheme, _ := themeBinding.Get(); currentTheme != selection {
			themeBinding.Set(selection)
			a.Settings().SetTheme(appTheme)
			refreshWindow(a, w)
		}
	})
	currentTheme, _ := themeBinding.Get()
	themeSelect.SetSelected(currentTheme)

	// Get list of embedded fonts
	fontSelections := []string{DEFAULT}
	fontEntries, _ := fsFonts.ReadDir("internal/fonts")
	for _, fe := range fontEntries {
		// FiraCode-Regular.ttf -> FiraCode
		if fbase, _, split := strings.Cut(fe.Name(), "-"); split {
			found := false
			for _, fs := range fontSelections {
				if fs == fbase {
					found = true
					break
				}
			}
			if !found {
				fontSelections = append(fontSelections, fbase)
			}
		}
	}

	fontBinding := binding.BindPreferenceString("font", a.Preferences())
	fontSelect := widget.NewSelect(fontSelections, func(selection string) {
		appTheme.fontName = selection
		if currentFont, _ := fontBinding.Get(); currentFont != selection {
			fontBinding.Set(selection)
			a.Settings().SetTheme(appTheme)
			refreshWindow(a, w)
		}
	})
	currentFont, _ := fontBinding.Get()
	fontSelect.SetSelected(currentFont)

	curveBinding := binding.BindPreferenceString("pake-curve", a.Preferences())

	curveSelect := widget.NewSelect(pake.AvailableCurves(), func(selection string) {
		curveBinding.Set(selection)
	})
	currentCurve, _ := curveBinding.Get()
	curveSelect.SetSelected(currentCurve)

	hashBinding := binding.BindPreferenceString("croc-hash", a.Preferences())
	hashSelect := widget.NewSelect([]string{"imohash", "md5", "xxhash", "highway"}, func(selection string) {
		hashBinding.Set(selection)
	})
	currentHash, _ := hashBinding.Get()
	hashSelect.SetSelected(currentHash)

	hideLogoBinding := binding.BindPreferenceBool("hide-logo", a.Preferences())
	toggleLogo := widget.NewCheck(lp("Hide"), nil)
	toggleLogo.Checked, _ = hideLogoBinding.Get()
	toggleLogo.OnChanged = func(bool) {
		hideLogo, _ := hideLogoBinding.Get()
		hideLogoBinding.Set(!hideLogo)
		refreshWindow(a, w)
	}

	// Настройки посредников
	relayAddressBinding := binding.BindPreferenceString("relay-address", a.Preferences())
	relay6Binding := binding.BindPreferenceString("relay6", a.Preferences())
	relayPortsBinding := binding.BindPreferenceString("relay-ports", a.Preferences())
	relayPasswordBinding := binding.BindPreferenceString("relay-password", a.Preferences())

	relaySocks5Binding := binding.BindPreferenceString("socks5", a.Preferences())
	relayConnectBinding := binding.BindPreferenceString("connect", a.Preferences())

	relayControls, relayUpdate := createRelaySelector(a, w,
		relayAddressBinding,
		relay6Binding,
		relayPortsBinding,
		relayPasswordBinding,
		relaySocks5Binding,
		relayConnectBinding,
	)

	// Создаем виджеты для полей
	relayAddressEntry := widget.NewEntryWithData(relayAddressBinding)
	relayAddressEntry.SetPlaceHolder("local")
	relay6Entry := widget.NewEntryWithData(relay6Binding)
	relayPortsEntry := widget.NewEntryWithData(relayPortsBinding)
	relayPortsEntry.SetPlaceHolder(ports0)
	relayPasswordEntry := widget.NewEntryWithData(relayPasswordBinding)
	relayPasswordEntry.SetPlaceHolder(DEFAULT_PASSPHRASE)

	relaySocks5Entry := widget.NewEntryWithData(relaySocks5Binding)
	relayConnectEntry := widget.NewEntryWithData(relayConnectBinding)

	disableLocalBinding := binding.BindPreferenceBool("disable-local", a.Preferences())
	disableLocalCheck := widget.NewCheckWithData(lp("Send only via relay"), disableLocalBinding)

	testingBinding := binding.BindPreferenceBool("testing", a.Preferences())
	testingCheck := widget.NewCheckWithData(lp("Ask the sender for their address"), testingBinding)

	onlyLocalBinding := binding.BindPreferenceBool("force-local", a.Preferences())
	onlyLocalCheck := widget.NewCheckWithData(lp("Connect to local senders only"), onlyLocalBinding)

	gitIgnoreBinding := binding.BindPreferenceBool("git", a.Preferences())
	gitIgnoreCheck := widget.NewCheckWithData(".gitignore", gitIgnoreBinding)

	overwriteBinding := binding.BindPreferenceBool("overwrite", a.Preferences())
	overwriteCheck := widget.NewCheckWithData(lp("Overwrite"), overwriteBinding)

	sendBinding := binding.BindPreferenceBool("send", a.Preferences())
	sendCheck := widget.NewCheckWithData("", sendBinding)
	sendCheck.OnChanged = func(ok bool) {
		sendBinding.Set(ok)
		json := "receive"
		if ok {
			json = "send"
		}
		json += ".json"
		sendCheck.SetText(json)
	}
	on, _ := sendBinding.Get()
	sendCheck.OnChanged(on)

	hostBinding := binding.BindPreferenceString("host", a.Preferences())
	prev := OFF

	ctx, ctc := context.WithCancel(appCtx)
	var hostSelect *Select
	hostSelect = NewSelect(hostSelectOptions(OFF), func(next string) {
		if next == prev {
			return
		}
		hostBinding.Set(next)
		if noRestart {
			if next == OFF {
				//Лучше использовать testing или ip или явно host чем флудить локалку
				// disableLocalBinding.Set(false)
				// disableLocalCheck.Refresh()
				prev = OFF
				ctc()
				return
			}
			if prev != OFF {
				// 192.168.0.1->0.0.0.0 не позволяем а опускаем 192.168.0.1
				hostSelect.SetSelected(OFF) // рекурсия
				return
			}
			ctx, ctc = context.WithCancel(appCtx)
		} else {
			if next == OFF {
				// disableLocalBinding.Set(false)
				// disableLocalCheck.Refresh()
				if prev != OFF {
					restart(w)
				}
				return
			}
			if prev != OFF {
				if next != prev {
					restart(w)
				}
				return
			}
		}
		var pass, host, ports string
		relay := getRelayByAddress(a, next)
		if relay.Name == "" {
			pass = DEFAULT_PASSPHRASE
			host = next
			ports = ports0
		} else {
			setRelayName(a, relay.Name)
			relayUpdate()
			pass = relay.Password
			host = relay.Address
			ports = relay.Ports
		}
		prev = host
		disableLocalBinding.Set(true)
		disableLocalCheck.Refresh()
		go func() {
			var err error
			if noRestart {
				err = relayRunCtx(ctx, w, pass, host, ports)
			} else {
				err = relayRun(w, pass, host, ports)
			}
			log.Debugf("relayRun: %v", err)
			// netstat -tlnp|grep crocgui
			// ss -tlnp|grep crocgui
			// netstat -a -n -p tcp |find ":90"
			fyne.Do(func() {
				hostSelect.SetSelected(OFF) // рекурсия
				// hostSelect.Refresh()
				if err != nil {
					NewToast(w, err.Error()).Show()
				}
			})
		}()
	})
	hostSelect.BeforePopup = func() {
		hostSelect.Options = hostSelectOptions(OFF)
		log.Debugf("Options %v", hostSelect.Options)
	}
	s, _ := hostBinding.Get()
	if s == "" {
		s = OFF
	}
	hostSelect.SetSelected(s)

	// Массив элементов для управления состоянием
	cosED := []fyne.CanvasObject{
		relayAddressEntry,
		relay6Entry,
		relayPortsEntry,
		relayPasswordEntry,
		disableLocalCheck,
		onlyLocalCheck,
		gitIgnoreCheck,
		overwriteCheck,
		curveSelect,
		hashSelect,
		relayControls,
	}

	var savedGuiPrefsData GuiPrefsData
	var guiSettingsSaved bool

	// Создаем restoreCheck после объявления всех переменных
	restoreBinding := binding.BindPreferenceBool("restore", a.Preferences())
	restoreCheck := widget.NewCheckWithData(lp("Restore"), restoreBinding)

	// Создаем map для удобного доступа к привязкам по ключу (json тегу)
	prefBindings := map[string]interface{}{
		"relay-address":  relayAddressBinding,
		"relay6":         relay6Binding,
		"relay-ports":    relayPortsBinding,
		"relay-password": relayPasswordBinding,
		"disable-local":  disableLocalBinding,
		"force-local":    onlyLocalBinding,
		"pake-curve":     curveBinding,
		"croc-hash":      hashBinding,
		"git":            gitIgnoreBinding,
		"overwrite":      overwriteBinding,
	}

	// Создаём формы для каждой секции
	// 1. Секция Appearance
	appearanceForm := widget.NewForm(
		widget.NewFormItem(lp("Language"), langSelect),
		widget.NewFormItem(lp("Theme"), themeSelect),
		widget.NewFormItem(lp("Font"), fontSelect),
		widget.NewFormItem(lp("Logo"), toggleLogo),
	)

	// 2. Секция Croc Config
	crocForm := widget.NewForm(
		widget.NewFormItem(lp("Configs"), container.NewHBox(
			widget.NewLabel(".config/croc/"),
			layout.NewSpacer(),
			widget.NewCheckWithData("remember", binding.BindPreferenceBool("remember", a.Preferences())),
		)),
		widget.NewFormItem(lp("Config"), container.NewHBox(
			sendCheck,
			layout.NewSpacer(),
			restoreCheck,
		)),
	)

	// 3. Секция Relay Settings
	env := []string{
		"$CROC_RELAY ",
		"$CROC_RELAY6",
		"$CROC_PASS " + lp("Value may be file with value"),
		"$SOCKS5_PROXY",
		"$HTTP_PROXY",
	}
	if isMobile || asMobile {
		env = make([]string, len(env))
	}
	ip := &widget.FormItem{
		Text:     "",
		Widget:   relayAddressEntry,
		HintText: env[0] + lp("If value like 0IP then it used as --ip IP"),
	}

	relayForm := widget.NewForm(
		widget.NewFormItem(lp("Name"), relayControls),
		ip,
		// widget.NewFormItem("relay6", relay6Entry),
		&widget.FormItem{
			Text:     "relay6",
			Widget:   relay6Entry,
			HintText: env[1],
		},
		widget.NewFormItem("ports", relayPortsEntry),
		// widget.NewFormItem("pass", relayPasswordEntry),
		&widget.FormItem{
			Text:     "pass",
			Widget:   relayPasswordEntry,
			HintText: env[2],
		},
		&widget.FormItem{
			Text:     "socks5",
			Widget:   relaySocks5Entry,
			HintText: env[3],
		},
		&widget.FormItem{
			Text:     "connect",
			Widget:   relayConnectEntry,
			HintText: env[4],
		},
	)

	relayAddressEntry.OnChanged = func(ra string) {
		relayAddressBinding.Set(ra)
		text := "relay"
		if strings.HasPrefix(ra, "0") {
			text = "ip"
		}
		if text != ip.Text {
			ip.Text = text
			de.Bounce(relayForm.Refresh)
		}
		// Обновляем опции селекта при изменении адреса релея
		hostSelect.Options = hostSelectOptions(OFF)
		hostSelect.Refresh()
	}
	ra, _ := relayAddressBinding.Get()
	relayAddressEntry.OnChanged(ra)

	// 4. Секция Network Local
	networkForm := widget.NewForm(
		widget.NewFormItem("host", hostSelect),
		widget.NewFormItem("no-local", disableLocalCheck),
		widget.NewFormItem("testing", testingCheck),
		widget.NewFormItem("local", onlyLocalCheck),
		widget.NewFormItem("multicast", widget.NewEntryWithData(binding.BindPreferenceString("multicast-address", a.Preferences()))),
	)

	// 5. Секция Storage Options
	s = lp("UnZip files")
	if isMobile || asMobile {
		s = lp("Zip folders")
	}

	storageForm := widget.NewForm(
		widget.NewFormItem("overwrite", overwriteCheck),
		widget.NewFormItem("git", gitIgnoreCheck),
		widget.NewFormItem("zip", widget.NewCheckWithData(s, binding.BindPreferenceBool("zip-unzip", a.Preferences()))),
		&widget.FormItem{
			Text:     "exclude",
			Widget:   widget.NewEntryWithData(binding.BindPreferenceString("exclude", a.Preferences())),
			HintText: lp("File path CSV parts"),
		},
	)

	// 6. Секция Transfer Options
	transferForm := widget.NewForm(
		widget.NewFormItem("curve", curveSelect),
		widget.NewFormItem("hash", hashSelect),
		widget.NewFormItem("no-multi", widget.NewCheckWithData(lp("Disable Multiplexing"), binding.BindPreferenceBool("disable-multiplexing", a.Preferences()))),
		widget.NewFormItem("no-compress", widget.NewCheckWithData(lp("Disable Compression"), binding.BindPreferenceBool("disable-compression", a.Preferences()))),
		// widget.NewFormItem("throttleUpload", widget.NewEntryWithData(binding.BindPreferenceString("upload-throttle", a.Preferences()))),
		&widget.FormItem{
			Text:     "throttleUpload",
			Widget:   widget.NewEntryWithData(binding.BindPreferenceString("upload-throttle", a.Preferences())),
			HintText: lp("speed e.g. 500k"),
		},
	)

	// Создаем основной контейнер для QR секции
	qr = NewQR(a, w)

	// Создаём аккордеон
	accordion = widget.NewAccordion(
		widget.NewAccordionItem(lp("Appearance"), appearanceForm),
		widget.NewAccordionItem("Croc", crocForm),
		widget.NewAccordionItem(lp("Relay"), relayForm),
		widget.NewAccordionItem(lp("Network Local"), networkForm),
		widget.NewAccordionItem(lp("Storage Options"), storageForm),
		widget.NewAccordionItem(lp("Transfer Options"), transferForm),
		qr.GetAccordionItem(),
	)
	accordion.MultiOpen = !(isMobile || asMobile)
	restoreAccordionState()

	// Собираем финальный интерфейс
	ti = container.NewTabItemWithIcon("", theme.SettingsIcon(), container.NewVScroll(
		container.NewVBox(accordion),
	))
	OnSelectedTab[SETTINGSi] = func() {
		qr.UpdateFromClipboard()
		ti.Content.Refresh()
	}

	restoreCheck.OnChanged = func(ok bool) {
		restoreBinding.Set(ok)
		if ok {
			// Сохраняем текущие значения привязок в структуру
			savedGuiPrefsData = saveBindingsToStruct(prefBindings)
			guiSettingsSaved = true

			// Отключаем элементы
			allEnabled(false, cosED...)
			accordion.Refresh()

			// Загружаем настройки из файла и применяем к привязкам
			send, _ := sendBinding.Get()
			if err := loadAndApplyCliOptionsToBindings(prefBindings, send); err != nil {
				log.Errorf("Failed to load settings: %v", err)
				NewToast(w, err.Error()).Show()
				// При ошибке отключаем чекбокс и возвращаем GUI
				restoreBinding.Set(false)
				allEnabled(true, cosED...)
				return
			}
		} else {
			// Включаем элементы
			allEnabled(true, cosED...)
			accordion.Refresh()

			// Восстанавливаем сохраненные значения привязок из структуры
			if guiSettingsSaved {
				applyStructToBindings(savedGuiPrefsData, prefBindings)
				log.Info("GUI settings restored")
			}
		}
	}
	on, _ = restoreBinding.Get()
	restoreCheck.OnChanged(on)

	return
}

// saveBindingsToStruct извлекает значения из map привязок и сохраняет их в структуру GuiPrefsData.
func saveBindingsToStruct(bindings map[string]interface{}) GuiPrefsData {
	data := GuiPrefsData{}
	dataType := reflect.TypeOf(data)
	dataValue := reflect.ValueOf(&data).Elem()

	for i := 0; i < dataType.NumField(); i++ {
		field := dataType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			continue // Пропускаем поля без тега json
		}

		bindingObj, ok := bindings[jsonTag]
		if !ok {
			continue // Привязка для этого ключа не найдена
		}

		bindingValue := reflect.ValueOf(bindingObj)
		getMethod := bindingValue.MethodByName("Get")
		if !getMethod.IsValid() {
			continue // Метод Get не найден
		}

		// Вызываем Get() метод привязки
		results := getMethod.Call(nil)
		if len(results) != 2 || results[1].Interface() != nil { // [0] - значение, [1] - ошибка
			log.Errorf("Failed to get value for binding '%s'", jsonTag)
			continue
		}
		bindingValueResult := results[0].Interface()

		// Устанавливаем значение в поле структуры
		fieldValue := dataValue.Field(i)
		if fieldValue.CanSet() {
			fieldValue.Set(reflect.ValueOf(bindingValueResult))
		}
	}

	return data
}

// applyStructToBindings устанавливает значения в map привязок из структуру GuiPrefsData.
func applyStructToBindings(data GuiPrefsData, bindings map[string]interface{}) {
	dataType := reflect.TypeOf(data)
	dataValue := reflect.ValueOf(data)

	for i := 0; i < dataType.NumField(); i++ {
		field := dataType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			continue // Пропускаем поля без тега json
		}

		bindingObj, ok := bindings[jsonTag]
		if !ok {
			continue // Привязка для этого ключа не найдена
		}

		fieldValue := dataValue.Field(i).Interface() // Получаем значение поля структуры

		bindingValue := reflect.ValueOf(bindingObj)
		setMethod := bindingValue.MethodByName("Set")
		if !setMethod.IsValid() {
			continue // Метод Set не найден
		}

		// Вызываем Set(value) метод привязки
		setMethod.Call([]reflect.Value{reflect.ValueOf(fieldValue)})
	}
}

// loadAndApplyCliOptionsToBindings загружает JSON из файла и применяет значения к map привязок.
func loadAndApplyCliOptionsToBindings(bindings map[string]interface{}, save bool) error {
	var options croc.Options
	b, err := os.ReadFile(getConfigFile(false, save))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &options); err != nil {
		return err
	}

	// Создаем GuiPrefsData из загруженных options
	if options.RelayAddress == DEFAULT {
		options.RelayAddress = DEFAULT_RELAY
	}
	if options.RelayAddress6 == DEFAULT {
		options.RelayAddress6 = DEFAULT_RELAY6
	}
	data := GuiPrefsData{
		RelayAddress:  strings.TrimPrefix(options.RelayAddress, NONDEFAULT),
		RelayAddress6: strings.TrimPrefix(options.RelayAddress6, NONDEFAULT),
		RelayPorts:    strings.Join(options.RelayPorts, ","),
		RelayPassword: options.RelayPassword,
		DisableLocal:  options.DisableLocal,
		OnlyLocal:     options.OnlyLocal,
		Curve:         options.Curve,
		HashAlgorithm: options.HashAlgorithm,
		GitIgnore:     options.GitIgnore,
		Overwrite:     options.Overwrite,
	}

	// Применяем данные к привязкам через map
	applyStructToBindings(data, bindings)

	return nil
}

// Сохраняем индексы открытых секций
func saveAccordionState() {
	if accordion == nil {
		return
	}
	var openIndices []int
	for i, item := range accordion.Items {
		if item.Open {
			openIndices = append(openIndices, i)
			if !accordion.MultiOpen {
				break
			}
		}
	}
	// Сохраняем как список интов
	fyne.CurrentApp().Preferences().SetIntList("accordion", openIndices)
	// log.Debugf("saveAccordionState %v", openIndices)
}

// Восстанавливаем открытые секции
func restoreAccordionState() {
	if accordion == nil {
		return
	}
	openIndices := fyne.CurrentApp().Preferences().IntList("accordion")
	// log.Debugf("restoreAccordionState %v", openIndices)

	for _, idx := range openIndices {
		if idx >= 0 && idx < len(accordion.Items) {
			accordion.Open(idx)
			if !accordion.MultiOpen {
				break
			}
		}
	}
}

// Функция для получения списка локальных IP-адресов
func localIPs() ([]string, error) {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		if ip := utils.LocalIP(); ip != "" {
			return []string{ip}, nil
		}
		// log.Errorf("interfaces %v", err)
		// conn, err := net.Dial("udp4", net.JoinHostPort(DEFAULT_RELAY, strconv.Itoa(DEFAULT_PORT)))
		// if err != nil {
		// 	return ips, err
		// }
		// defer conn.Close()
		// localAddr := conn.LocalAddr().(*net.UDPAddr)
		// ips = append(ips, localAddr.IP.String())
		// return ips, nil
		return ips, err
	}

	for _, iface := range interfaces {
		// Пропускаем неактивные интерфейсы
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			log.Errorf("Addrs %v", err)
			continue
		}

		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Пропускаем loopback и IPv6 (если нужно только IPv4)
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			// Для IPv4
			if ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}

	return ips, nil
}

// Функция для обновления опций селекта
func hostSelectOptions(first string) []string {
	ips, err := localIPs()
	if err != nil {
		log.Errorf("%v", err)
	}
	options := []string{first}

	if err == nil && len(ips) > 0 {
		options = append(options, ips...)
	}
	if len(options) > 2 && first == OFF {
		options = append(options, ALL)
	}

	return options
}
