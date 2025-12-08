// settings.go
package main

import (
	"embed"
	"encoding/json"
	"os"
	"reflect"
	"strings"

	"github.com/schollz/croc/v10/src/croc"
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

func settingsTabItem(a fyne.App, w fyne.Window) (ti *container.TabItem) {
	langBinding := binding.BindPreferenceString("lang", a.Preferences())
	langSelect := widget.NewSelect([]string{"en-US", "tr-TR", "ja-JP", "zh-CN", "zh-HK", "zh-TW", "ru-RU"}, func(selection string) {
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
	toggleLogo := widget.NewButton(lp("Show / Hide"), func() {
		hideLogo, _ := hideLogoBinding.Get()
		hideLogoBinding.Set(!hideLogo)
		refreshWindow(a, w)
	})

	s := lp("Zip folders")
	if !(isMobile || asMobile) {
		s += " / " + lp("UnZip files")
	}

	// Настройки посредников
	relayAddressBinding := binding.BindPreferenceString("relay-address", a.Preferences())
	relay6Binding := binding.BindPreferenceString("relay6", a.Preferences())
	relayPortsBinding := binding.BindPreferenceString("relay-ports", a.Preferences())
	relayPasswordBinding := binding.BindPreferenceString("relay-password", a.Preferences())
	relayControls := createRelaySelector(a, w,
		relayAddressBinding,
		relay6Binding,
		relayPortsBinding, relayPasswordBinding)

	// Создаем виджеты для полей
	relayAddressEntry := widget.NewEntryWithData(relayAddressBinding)
	relay6Entry := widget.NewEntryWithData(relay6Binding)
	relayPortsEntry := widget.NewEntryWithData(relayPortsBinding)
	relayPasswordEntry := widget.NewEntryWithData(relayPasswordBinding)

	disableLocalBinding := binding.BindPreferenceBool("disable-local", a.Preferences())
	disableLocalCheck := widget.NewCheckWithData("", disableLocalBinding)

	onlyLocalBinding := binding.BindPreferenceBool("force-local", a.Preferences())
	onlyLocalCheck := widget.NewCheckWithData("", onlyLocalBinding)

	gitIgnoreBinding := binding.BindPreferenceBool("git", a.Preferences())
	gitIgnoreCheck := widget.NewCheckWithData("", gitIgnoreBinding)

	overwriteBinding := binding.BindPreferenceBool("overwrite", a.Preferences())
	overwriteCheck := widget.NewCheckWithData("", overwriteBinding)

	sendBinding := binding.BindPreferenceBool("send", a.Preferences())

	sendCheck := widget.NewCheckWithData("", sendBinding)
	sendCheck.OnChanged = func(send bool) {
		json := "receive"
		if send {
			json = "send"
		}
		json += ".json"
		sendCheck.SetText(json)
		sendBinding.Set(send)
	}
	send, _ := sendBinding.Get()
	sendCheck.OnChanged(send)

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

	ti = container.NewTabItemWithIcon(ZeroWidthNonJoiner, theme.SettingsIcon(), container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle(lp("Appearance"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("Language"), langSelect),
			widget.NewFormItem(lp("Theme"), themeSelect),
			widget.NewFormItem(lp("Font"), fontSelect),
			widget.NewFormItem(lp("Logo"), toggleLogo),
		),
		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem(lp("Configs"), container.NewHBox(
				widget.NewLabel(".config/croc/"),
				layout.NewSpacer(),
				widget.NewCheckWithData(lp("Remember"),
					binding.BindPreferenceBool("remember", a.Preferences())),
			)),
			widget.NewFormItem(lp("Config"), container.NewHBox(
				sendCheck,
				layout.NewSpacer(),
				restoreCheck,
			)),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Relay"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("Name"), relayControls),
			widget.NewFormItem(lp("Address"), relayAddressEntry),
			widget.NewFormItem(lp("Address6"), relay6Entry),
			widget.NewFormItem(lp("Ports"), relayPortsEntry),
			widget.NewFormItem(lp("Password"), relayPasswordEntry),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Network Local"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("Disable Local"), disableLocalCheck),
			widget.NewFormItem(lp("Force Local Only"), onlyLocalCheck),
			widget.NewFormItem(lp("Multicast Address"), widget.NewEntryWithData(binding.BindPreferenceString("multicast-address", a.Preferences()))),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Transfer Options"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("PAKE Curve"), curveSelect),
			widget.NewFormItem(lp("Hash Algorithm"), hashSelect),
			widget.NewFormItem(lp("Disable Multiplexing"), widget.NewCheckWithData("", binding.BindPreferenceBool("disable-multiplexing", a.Preferences()))),
			widget.NewFormItem(lp("Disable Compression"), widget.NewCheckWithData("", binding.BindPreferenceBool("disable-compression", a.Preferences()))),
			widget.NewFormItem(lp("Upload Speed Throttle"), widget.NewEntryWithData(binding.BindPreferenceString("upload-throttle", a.Preferences()))),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(lp("Storage Options"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem(lp("Overwrite"), overwriteCheck),
			widget.NewFormItem(lp("GitIgnore"), gitIgnoreCheck),
			widget.NewFormItem(s, widget.NewCheckWithData("", binding.BindPreferenceBool("zip-unzip", a.Preferences()))),
			widget.NewFormItem(lp("Exclude"), widget.NewEntryWithData(binding.BindPreferenceString("exclude", a.Preferences()))),
		),
	)))

	restoreCheck.OnChanged = func(restore bool) {
		restoreBinding.Set(restore)
		if restore {
			// Сохраняем текущие значения привязок в структуру
			savedGuiPrefsData = saveBindingsToStruct(prefBindings)
			guiSettingsSaved = true

			// Отключаем элементы
			allEnabled(false, cosED...)
			ti.Content.Refresh()

			// Загружаем настройки из файла и применяем к привязкам
			send, _ := sendBinding.Get()
			if err := loadAndApplyCliOptionsToBindings(prefBindings, send); err != nil {
				log.Errorf("Failed to load settings: %v", err)
				NewToast(w, err.Error()).Show()
				// При ошибке отключаем чекбокс и возвращаем GUI
				// restoreCheck.SetChecked(false)
				restoreBinding.Set(false)
				allEnabled(true, cosED...)
				return
			}
		} else {
			// Включаем элементы
			allEnabled(true, cosED...)
			ti.Content.Refresh()

			// Восстанавливаем сохраненные значения привязок из структуры
			if guiSettingsSaved {
				applyStructToBindings(savedGuiPrefsData, prefBindings)
				log.Info("GUI settings restored")
			}
		}
	}
	restored, _ := restoreBinding.Get()
	restoreCheck.OnChanged(restored)

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

// applyStructToBindings устанавливает значения в map привязок из структуры GuiPrefsData.
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
