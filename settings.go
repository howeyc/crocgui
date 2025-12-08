// settings.go
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/schollz/croc/v10/src/croc"
	log "github.com/schollz/logger"
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

// Структура для хранения всех GUI настроек
type GuiSettings struct {
	RelayAddress  string
	RelayAddress6 string
	RelayPorts    []string
	RelayPassword string
	DisableLocal  bool
	OnlyLocal     bool
	Curve         string
	HashAlgorithm string
	GitIgnore     bool
	Overwrite     bool
}

func settingsTabItem(a fyne.App, w fyne.Window) *container.TabItem {
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
	curveSelect := widget.NewSelect([]string{"siec", "p256", "p348", "p521"}, func(selection string) {
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

	var savedGuiSettings GuiSettings
	var guiSettingsSaved bool

	// Создаем restoreCheck после объявления всех переменных
	restoreCheck := widget.NewCheckWithData(lp("Restore"), binding.BindPreferenceBool("restore", a.Preferences()))
	restoreCheck.OnChanged = func(restore bool) {
		if restore {
			// Сохраняем текущие GUI значения
			savedGuiSettings = saveCurrentGuiSettings(
				relayAddressBinding,  // String
				relay6Binding,        // String
				relayPortsBinding,    // String
				relayPasswordBinding, // String
				disableLocalBinding,  // Bool
				onlyLocalBinding,     // Bool
				curveBinding,         // String
				hashBinding,          // String
				gitIgnoreBinding,     // Bool
				overwriteBinding,     // Bool
			)
			guiSettingsSaved = true

			// Отключаем элементы
			allEnabled(false, cosED...)

			// Загружаем настройки из файла
			if err := loadAndApplyCliOptions(
				relayAddressBinding,  // String
				relay6Binding,        // String
				relayPortsBinding,    // String
				relayPasswordBinding, // String
				disableLocalBinding,  // Bool
				onlyLocalBinding,     // Bool
				curveBinding,         // String
				hashBinding,          // String
				gitIgnoreBinding,     // Bool
				overwriteBinding,     // Bool
				sendBinding,          // Bool
			); err != nil {
				log.Errorf("Failed to load settings: %v", err)
				NewToast(w, err.Error()).Show()
				// При ошибке отключаем чекбокс и возвращаем GUI
				restoreCheck.SetChecked(false)
				allEnabled(true, cosED...)
				return
			}
		} else {
			// Включаем элементы
			allEnabled(true, cosED...)

			// Восстанавливаем сохраненные GUI значения
			if guiSettingsSaved {
				restoreGuiSettings(savedGuiSettings,
					relayAddressBinding,  // String
					relay6Binding,        // String
					relayPortsBinding,    // String
					relayPasswordBinding, // String
					disableLocalBinding,  // Bool
					onlyLocalBinding,     // Bool
					curveBinding,         // String
					hashBinding,          // String
					gitIgnoreBinding,     // Bool
					overwriteBinding,     // Bool
				)
				log.Info("GUI settings restored")
			}
		}
	}

	return container.NewTabItemWithIcon(ZeroWidthNonJoiner, theme.SettingsIcon(), container.NewVScroll(container.NewVBox(
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
}

// Вспомогательные функции для сохранения и восстановления настроек

func saveCurrentGuiSettings(
	relayAddressBinding, relay6Binding, relayPortsBinding, relayPasswordBinding binding.String,
	disableLocalBinding, onlyLocalBinding binding.Bool,
	curveBinding, hashBinding binding.String,
	gitIgnoreBinding, overwriteBinding binding.Bool,
) GuiSettings {
	relayAddress, _ := relayAddressBinding.Get()
	relay6, _ := relay6Binding.Get()
	ports, _ := relayPortsBinding.Get()
	relayPassword, _ := relayPasswordBinding.Get()

	disableLocal, _ := disableLocalBinding.Get()
	onlyLocal, _ := onlyLocalBinding.Get()
	curve, _ := curveBinding.Get()
	hash, _ := hashBinding.Get()
	gitIgnore, _ := gitIgnoreBinding.Get()
	overwrite, _ := overwriteBinding.Get()

	return GuiSettings{
		RelayAddress:  relayAddress,
		RelayAddress6: relay6,
		RelayPorts:    strings.Split(ports, ","),
		RelayPassword: relayPassword,
		DisableLocal:  disableLocal,
		OnlyLocal:     onlyLocal,
		Curve:         curve,
		HashAlgorithm: hash,
		GitIgnore:     gitIgnore,
		Overwrite:     overwrite,
	}
}

func loadAndApplyCliOptions(
	relayAddressBinding, relay6Binding, relayPortsBinding, relayPasswordBinding binding.String,
	disableLocalBinding, onlyLocalBinding binding.Bool,
	curveBinding, hashBinding binding.String,
	gitIgnoreBinding, overwriteBinding, saveBinding binding.Bool,
) error {
	save, _ := saveBinding.Get()
	var options croc.Options
	b, err := os.ReadFile(getConfigFile(false, save))
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if err := json.Unmarshal(b, &options); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	// Применяем настройки из файла ко всем привязкам
	if options.RelayAddress == DEFAULT {
		options.RelayAddress = DEFAULT_RELAY
	} else {
		options.RelayAddress = strings.TrimPrefix(options.RelayAddress, NONDEFAULT)
	}
	relayAddressBinding.Set(options.RelayAddress)

	if options.RelayAddress6 == DEFAULT {
		options.RelayAddress6 = DEFAULT_RELAY6
	} else {
		options.RelayAddress6 = strings.TrimPrefix(options.RelayAddress6, NONDEFAULT)
	}
	relay6Binding.Set(options.RelayAddress6)

	relayPortsBinding.Set(strings.Join(options.RelayPorts, ","))
	relayPasswordBinding.Set(options.RelayPassword)

	// Применяем остальные настройки
	disableLocalBinding.Set(options.DisableLocal)
	onlyLocalBinding.Set(options.OnlyLocal)
	curveBinding.Set(options.Curve)
	hashBinding.Set(options.HashAlgorithm)
	gitIgnoreBinding.Set(options.GitIgnore)
	overwriteBinding.Set(options.Overwrite)

	return nil
}

func restoreGuiSettings(settings GuiSettings,
	relayAddressBinding, relay6Binding, relayPortsBinding, relayPasswordBinding binding.String,
	disableLocalBinding, onlyLocalBinding binding.Bool,
	curveBinding, hashBinding binding.String,
	gitIgnoreBinding, overwriteBinding binding.Bool,
) {
	relayAddressBinding.Set(settings.RelayAddress)
	relay6Binding.Set(settings.RelayAddress6)
	relayPortsBinding.Set(strings.Join(settings.RelayPorts, ","))
	relayPasswordBinding.Set(settings.RelayPassword)
	disableLocalBinding.Set(settings.DisableLocal)
	onlyLocalBinding.Set(settings.OnlyLocal)
	curveBinding.Set(settings.Curve)
	hashBinding.Set(settings.HashAlgorithm)
	gitIgnoreBinding.Set(settings.GitIgnore)
	overwriteBinding.Set(settings.Overwrite)
}
