// relays.go
package main

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var relayUpdateUI func()

// addCurrentRelay сохраняет текущие настройки посредника из preferences в список профилей
func addCurrentRelay(a fyne.App) {
	name := strings.TrimSpace(a.Preferences().String("new-relay"))
	if name == "" {
		return
	}
	relays := getRelays(a)
	relay, index := relayByName(relays, name)
	relay.Name = name
	relay.Address = a.Preferences().String("relay-address")
	relay.Address6 = a.Preferences().String("relay6")
	relay.Ports = a.Preferences().String("relay-ports")
	// relay.Password = a.Preferences().String("relay-password")
	// relay.Socks5 = a.Preferences().String("socks5")
	// relay.Connect = a.Preferences().String("connect")
	if index < 0 {
		relays = append(relays, relay)
	} else {
		relays[index] = relay
	}
	saveRelays(a, relays)
	setRelayName(a, name)
	if relayUpdateUI != nil {
		relayUpdateUI()
	}
}

// Relay представляет посредника
type Relay struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Address6 string `json:"address6"`
	Ports    string `json:"ports"`
	Password string `json:"password"`
	Socks5   string `json:"socks4"`
	Connect  string `json:"connect"`
}

// Ключи для хранения посредников в настройках
const (
	relaysKey = "relay-profiles"
	relayKey  = "current-relay"
)

// getRelays возвращает список посредников из настроек
func getRelays(a fyne.App) []Relay {
	relaysJSON := a.Preferences().String(relaysKey)
	defaultRelays := []Relay{
		{
			Name:     DEFAULT,
			Address:  a.Preferences().String("relay-address"),
			Address6: a.Preferences().String("relay6"),
			Ports:    a.Preferences().String("relay-ports"),
			Password: a.Preferences().String("relay-password"),
		},
	}
	if relaysJSON == "" {
		// Используем текущие значения из настроек для создания дефолтного профиля
		return defaultRelays
	}

	var relays []Relay
	if json.Unmarshal([]byte(relaysJSON), &relays) == nil {
		return relays
	}
	return defaultRelays
}

// saveRelays сохраняет посредников в relaysKey
func saveRelays(a fyne.App, relays []Relay) error {
	relaysJSON, err := json.Marshal(relays)
	if err != nil {
		return err
	}
	a.Preferences().SetString(relaysKey, string(relaysJSON))
	return nil
}

// relayByName ищет посредника по имени
func relayByName(relays []Relay, name string) (Relay, int) {
	for i, relay := range relays {
		if relay.Name == name {
			return relay, i
		}
	}
	return Relay{}, -1
}

// setRelayName обновляет текущего посредника в настройках
func setRelayName(a fyne.App, relayName string) {
	a.Preferences().SetString(relayKey, relayName)
}

// relayName возвращает имя текущего выбранного посредника
func relayName(a fyne.App) string {
	return a.Preferences().StringWithFallback(relayKey, DEFAULT)
}

func createRelaySelector(a fyne.App, w fyne.Window,
	addressBinding,
	address6Binding,
	portsBinding,
	passwordBinding,
	socks5Binding,
	connectBinding binding.String) (relayControls *fyne.Container, updateRelaySelector func()) {

	var (
		relaySelect *widget.Select
		nameEntry   *widget.Entry
	)
	// updateRelayValues обновляет значения полей на основе выбранного посредника
	updateRelayValues := func(relay Relay) {
		addressBinding.Set(relay.Address)
		address6Binding.Set(relay.Address6)
		portsBinding.Set(relay.Ports)
		passwordBinding.Set(relay.Password)
		socks5Binding.Set(relay.Socks5)
		connectBinding.Set(relay.Connect)
	}
	// Функция для обновления комбобокса из актуальных данных
	updateRelaySelector = func() {
		relays := getRelays(a)

		// Создаем список имен посредников
		relayNames := make([]string, len(relays))
		for i, relay := range relays {
			relayNames[i] = relay.Name
		}
		slices.Sort(relayNames)

		// Обновляем опции комбобокса
		relaySelect.Options = relayNames

		// Устанавливаем текущего посредника
		currentRelayName := relayName(a)

		switch {
		case currentRelayName != "":
			if relay, index := relayByName(relays, currentRelayName); index >= 0 {
				relaySelect.SetSelected(relay.Name)
				updateRelayValues(relay)
				break
			}
			fallthrough // Если не найден, переходим к случаю ниже
		case len(relays) > 0:
			relaySelect.SetSelected(relays[0].Name)
			updateRelayValues(relays[0])
			setRelayName(a, relays[0].Name)
		}

		relaySelect.Refresh()
	}

	// Создаем комбобокс
	relaySelect = widget.NewSelect([]string{}, func(selection string) {
		if selection != "" {
			if relay, index := relayByName(getRelays(a), selection); index >= 0 {
				updateRelayValues(relay)
				setRelayName(a, selection)
			}
		}
	})

	// Создаем поле ввода для имени нового посредника
	nameBinding := binding.BindPreferenceString("new-relay", a.Preferences())
	nameEntry = widget.NewEntryWithData(nameBinding)
	nameEntry.SetText("")

	// Функция добавления/обновления посредника
	addRelay := func() {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			name = strings.TrimSpace(relayName(a))
		}
		if name == "" {
			NewToast(w, lp("Empty name")).Show()
			return
		}

		// Загружаем текущие релеи
		relays := getRelays(a)
		relay, index := relayByName(relays, name)
		relay.Address, _ = addressBinding.Get()
		relay.Address6, _ = address6Binding.Get()
		relay.Ports, _ = portsBinding.Get()
		relay.Password, _ = passwordBinding.Get()
		relay.Socks5, _ = socks5Binding.Get()
		relay.Connect, _ = connectBinding.Get()

		if index < 0 {
			relay.Name = name
			relays = append(relays, relay)
		} else {
			relays[index] = relay
		}

		if err := saveRelays(a, relays); err != nil {
			NewToast(w, err.Error()).Show()
			return
		}

		// Обновляем UI
		setRelayName(a, name)
		updateRelaySelector()
		nameEntry.SetText("")
		NewToast(w, "Ok").Show()
		// setClipboard("", a)
	}

	// Обработка нажатия Enter в поле ввода
	nameEntry.OnSubmitted = func(text string) {
		addRelay()
	}

	// Кнопка для добавления нового посредника
	addRelayBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), addRelay)

	nameEntry.OnChanged = func(name string) {
		if nameEntry.TextStyle.Italic {
			nameEntry.Undo()
			return
		}
		if strings.TrimSpace(name) == "" {
			addRelayBtn.SetIcon(theme.ViewRefreshIcon())
			return
		}

		if _, index := relayByName(getRelays(a), name); index >= 0 {
			addRelayBtn.SetIcon(theme.ViewRefreshIcon())
			return
		}

		addRelayBtn.SetIcon(theme.ContentAddIcon())
	}

	// Функция удаления текущего посредника
	deleteRelay := func() {
		currentRelay := relaySelect.Selected
		if currentRelay == "" {
			NewToast(w, lp("Empty name")).Show()
			return
		}

		// Загружаем текущие релеи
		relays := getRelays(a)
		if len(relays) < 2 {
			NewToast(w, lp("Last item of list")).Show()
			return
		}

		// Находим и удаляем посредника
		_, index := relayByName(relays, currentRelay)
		if index < 0 {
			return
		}

		relays = append(relays[:index], relays[index+1:]...)
		if err := saveRelays(a, relays); err != nil {
			NewToast(w, err.Error()).Show()
			return
		}

		// Обновляем UI
		updateRelaySelector()
	}

	// Кнопка для удаления текущего посредника
	deleteRelayBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), deleteRelay)

	// Первоначальная загрузка
	relayUpdateUI = updateRelaySelector
	updateRelaySelector()
	relayControls = container.NewBorder(
		nil, nil,
		container.NewHBox(
			container.NewGridWrap(widget.NewLabel("\t\t").MinSize(), relaySelect),
			deleteRelayBtn),
		addRelayBtn,
		nameEntry,
	)
	return
}

func getRelayByAddress(a fyne.App, targetAddress string) (relay Relay) {
	relays := getRelays(a)

	// Внутренняя функция для очистки адреса от порта
	cleanAddress := func(addr string) string {
		if idx := strings.LastIndex(addr, ":"); idx != -1 {
			portPart := addr[idx+1:]
			if _, err := strconv.Atoi(portPart); err == nil {
				return addr[:idx]
			}
		}
		return addr
	}

	// Очищаем targetAddress от порта
	cleanTargetAddress := cleanAddress(targetAddress)

	// Ищем посредника
	for _, relay := range relays {
		// Очищаем адрес посредника для сравнения
		cleanRelayAddress := cleanAddress(relay.Address)

		// Сравниваем очищенные адреса
		if cleanRelayAddress == cleanTargetAddress {
			return relay
		}
	}

	return
}
