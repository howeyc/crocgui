// relays.go
package main

import (
	"encoding/json"
	"errors"
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
func updateRelayValues(relay Relay, addressBinding, portsBinding, passwordBinding binding.String) {
	addressBinding.Set(relay.Address)
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
	addressBinding, portsBinding, passwordBinding binding.String) (rs *fyne.Container) {

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
				updateRelayValues(relay, addressBinding, portsBinding, passwordBinding)
			} else if len(relays) > 0 {
				relaySelect.SetSelected(relays[0].Name)
				updateRelayValues(relays[0], addressBinding, portsBinding, passwordBinding)
				setRelayName(a, relays[0].Name)
			}
		} else if len(relays) > 0 {
			relaySelect.SetSelected(relays[0].Name)
			updateRelayValues(relays[0], addressBinding, portsBinding, passwordBinding)
			setRelayName(a, relays[0].Name)
		}

		relaySelect.Refresh()
	}

	// Создаем комбобокс
	relaySelect = widget.NewSelect([]string{}, func(selection string) {
		if selection != "" {
			if relay, index := relayByName(getRelays(a), selection); index >= 0 {
				updateRelayValues(relay, addressBinding, portsBinding, passwordBinding)
				setRelayName(a, selection)
			}
		}
	})

	// Создаем поле ввода для имени нового посредника
	nameEntry = widget.NewEntry()
	nameEntry.SetText(" ")

	// Валидатор для проверки уникальности имени
	nameEntry.Validator = func(text string) error {
		if text == "" {
			return nil // Пустое поле - это нормально для placeholder
		}

		// Проверяем, существует ли уже посредник с таким именем
		if _, index := relayByName(getRelays(a), text); index >= 0 {
			s := lp("Name already exists")
			return errors.New(s)
		}

		return nil
	}

	// Функция добавления нового посредника
	addRelay := func() {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			NewToast(w, lp("Empty name")).Show()
			return
		}

		// Проверяем валидацию
		if err := nameEntry.Validator(name); err != nil {
			NewToast(w, err.Error()).Show()
			return
		}

		// Получаем текущие значения полей
		currentAddress, _ := addressBinding.Get()
		currentPorts, _ := portsBinding.Get()
		currentPassword, _ := passwordBinding.Get()

		// Загружаем текущие релеи
		relays := getRelays(a)

		// Создаем нового посредника
		newRelay := Relay{
			Name:     name,
			Address:  currentAddress,
			Ports:    currentPorts,
			Password: currentPassword,
		}

		// Добавляем и сохраняем
		relays = append(relays, newRelay)
		if err := saveRelays(a, relays); err != nil {
			NewToast(w, err.Error()).Show()
			return
		}

		// Обновляем UI
		updateRelaySelector()
		setRelayName(a, name)
		nameEntry.SetText(" ")
		NewToast(w, "Ok").Show()
	}

	// Обработка нажатия Enter в поле ввода
	nameEntry.OnSubmitted = func(text string) {
		addRelay()
	}

	// Кнопка для добавления нового посредника
	addRelayBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), addRelay)

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

	rs = container.NewBorder(
		nil, nil,
		container.NewHBox(
			container.NewGridWrap(fyne.NewSize(92, relaySelect.MinSize().Height), relaySelect),
			deleteRelayBtn),
		addRelayBtn,
		nameEntry,
	)
	return
}
