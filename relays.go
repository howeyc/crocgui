// relays.go
package main

import (
	"encoding/json"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Relay представляет посредника
type Relay struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Address6 string `json:"address6"`
	Ports    string `json:"ports"`
	Password string `json:"password"`
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

// updateRelayValues обновляет значения полей на основе выбранного посредника
func updateRelayValues(relay Relay,
	addressBinding,
	address6Binding,
	portsBinding,
	passwordBinding binding.String) {
	addressBinding.Set(relay.Address)
	address6Binding.Set(relay.Address6)
	portsBinding.Set(relay.Ports)
	passwordBinding.Set(relay.Password)
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
	passwordBinding binding.String) (relayControls *fyne.Container) {

	var (
		relaySelect *widget.Select
		nameEntry   *widget.Entry
	)

	// Функция для обновления комбобокса из актуальных данных
	updateRelaySelector := func() {
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
		if currentRelayName != "" {
			if relay, index := relayByName(relays, currentRelayName); index >= 0 {
				relaySelect.SetSelected(relay.Name)
				updateRelayValues(relay,
					addressBinding,
					address6Binding,
					portsBinding, passwordBinding)
			} else if len(relays) > 0 {
				relaySelect.SetSelected(relays[0].Name)
				updateRelayValues(relays[0],
					addressBinding,
					address6Binding,
					portsBinding, passwordBinding)
				setRelayName(a, relays[0].Name)
			}
		} else if len(relays) > 0 {
			relaySelect.SetSelected(relays[0].Name)
			updateRelayValues(relays[0],
				addressBinding,
				address6Binding,
				portsBinding, passwordBinding)
			setRelayName(a, relays[0].Name)
		}

		relaySelect.Refresh()
	}

	// Создаем комбобокс
	relaySelect = widget.NewSelect([]string{}, func(selection string) {
		if selection != "" {
			if relay, index := relayByName(getRelays(a), selection); index >= 0 {
				updateRelayValues(relay,
					addressBinding,
					address6Binding,
					portsBinding, passwordBinding)
				setRelayName(a, selection)
			}
		}
	})

	// Создаем поле ввода для имени нового посредника
	nameEntry = widget.NewEntry()
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

		// Получаем текущие значения полей
		address, _ := addressBinding.Get()
		address6, _ := address6Binding.Get()
		ports, _ := portsBinding.Get()
		password, _ := passwordBinding.Get()

		// Загружаем текущие релеи
		relays := getRelays(a)
		relay, index := relayByName(relays, name)

		// Если не найден - создаем НОВЫЙ релей с указанным именем
		if index < 0 {
			relay = Relay{
				Name:     name,
				Address:  address,
				Address6: address6,
				Ports:    ports,
				Password: password,
			}
			relays = append(relays, relay)
		} else {
			// Обновляем существующий
			relay.Address = address
			relay.Address6 = address6
			relay.Ports = ports
			relay.Password = password
			relays[index] = relay
		}

		if err := saveRelays(a, relays); err != nil {
			NewToast(w, err.Error()).Show()
			return
		}

		// Обновляем UI
		updateRelaySelector()
		setRelayName(a, name)
		nameEntry.SetText("")
		NewToast(w, "Ok").Show()
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
	updateRelaySelector()
	label := widget.NewLabel("\t\t")
	relayControls = container.NewBorder(
		nil, nil,
		container.NewHBox(
			// container.NewGridWrap(fyne.NewSize(90, relaySelect.MinSize().Height), relaySelect),
			container.NewGridWrap(fyne.NewSize(label.MinSize().Width, label.MinSize().Height), relaySelect),
			deleteRelayBtn),
		addRelayBtn,
		nameEntry,
	)
	return
}
