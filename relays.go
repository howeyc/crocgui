// relays.go
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
)

// Relay представляет профиль настроек посредника
type Relay struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Ports    string `json:"ports"`
	Password string `json:"password"`
}

// Ключ для хранения профилей в настройках
const (
	relaysKey = "relay-profiles"
	relayKey  = "current-relay"
)

// getRelays возвращает список профилей из настроек
func getRelays(a fyne.App) ([]Relay, error) {
	relaysJSON := a.Preferences().String(relaysKey)
	if relaysJSON == "" {
		// Используем текущие значения из настроек для создания дефолтного профиля
		return []Relay{
			{
				Name:     DEFAULT,
				Address:  a.Preferences().String("relay-address"),
				Ports:    a.Preferences().String("relay-ports"),
				Password: a.Preferences().String("relay-password"),
			},
		}, nil
	}

	var relay []Relay
	err := json.Unmarshal([]byte(relaysJSON), &relay)
	return relay, err
}

// saveRelays сохраняет список профилей в настройки
func saveRelays(a fyne.App, profiles []Relay) error {
	relaysJSON, err := json.Marshal(profiles)
	if err != nil {
		return err
	}
	a.Preferences().SetString(relaysKey, string(relaysJSON))
	return nil
}

// findRelayByName ищет профиль по имени
func findRelayByName(relays []Relay, name string) (Relay, int) {
	for i, relay := range relays {
		if relay.Name == name {
			return relay, i
		}
	}
	return Relay{}, -1
}

// updateRelayValues обновляет значения полей на основе выбранного профиля
func updateRelayValues(profile Relay, addressBinding, portsBinding, passwordBinding binding.String) {
	addressBinding.Set(profile.Address)
	portsBinding.Set(profile.Ports)
	passwordBinding.Set(profile.Password)
}

// updateCurrentRelay обновляет текущего посредника в настройках
func updateCurrentRelay(a fyne.App, relayName string) {
	a.Preferences().SetString(relayKey, relayName)
}

// getCurrentRelayName возвращает имя текущего выбранного посредника
func getCurrentRelayName(a fyne.App) string {
	return a.Preferences().StringWithFallback(relayKey, DEFAULT)
}

func createRelaySelector0(a fyne.App, w fyne.Window,
	addressBinding, portsBinding, passwordBinding binding.String) *fyne.Container {

	// Комбобокс нужно вынести, чтобы иметь к нему доступ в замыканиях
	var relaySelect *widget.Select

	// Функция для обновления комбобокса из актуальных данных
	updateRelaySelector := func() {
		relays, err := getRelays(a)
		if err != nil {
			log.Tracef("Error loading relays: %v", err)
			relays = []Relay{}
		}

		// Создаем список имен посредников
		relayNames := make([]string, len(relays))
		for i, profile := range relays {
			relayNames[i] = profile.Name
		}

		// Обновляем опции комбобокса
		relaySelect.Options = relayNames

		// Устанавливаем текущего посредника
		currentRelayName := getCurrentRelayName(a)
		if currentRelayName != "" {
			if relay, index := findRelayByName(relays, currentRelayName); index >= 0 {
				relaySelect.SetSelected(relay.Name)
				updateRelayValues(relay, addressBinding, portsBinding, passwordBinding)
			} else if len(relays) > 0 {
				relaySelect.SetSelected(relays[0].Name)
				updateRelayValues(relays[0], addressBinding, portsBinding, passwordBinding)
				updateCurrentRelay(a, relays[0].Name)
			}
		} else if len(relays) > 0 {
			relaySelect.SetSelected(relays[0].Name)
			updateRelayValues(relays[0], addressBinding, portsBinding, passwordBinding)
			updateCurrentRelay(a, relays[0].Name)
		}

		relaySelect.Refresh()
	}

	// Создаем комбобокс с обработчиком
	relaySelect = widget.NewSelect([]string{}, func(selection string) {
		if selection != "" {
			relays, err := getRelays(a)
			if err == nil {
				if relay, index := findRelayByName(relays, selection); index >= 0 {
					updateRelayValues(relay, addressBinding, portsBinding, passwordBinding)
					updateCurrentRelay(a, selection)
				}
			}
		}
	})

	// Первоначальная загрузка
	updateRelaySelector()

	// Кнопка для добавления нового посредника (упрощенная версия)
	addRelayBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		nameEntry := widget.NewEntry()

		dialog.ShowForm(lp("New relay"), lp("Save"), lp("Cancel"), []*widget.FormItem{
			widget.NewFormItem(lp("Relay name"), nameEntry),
		}, func(b bool) {
			if !b || nameEntry.Text == "" {
				return
			}

			// Загружаем актуальные релеи
			relays, err := getRelays(a)
			if err != nil {
				NewToast(w, "Error loading relays: "+err.Error()).Show()
				return
			}

			// Проверяем существование
			if _, index := findRelayByName(relays, nameEntry.Text); index >= 0 {
				NewToast(w, lp("A relay with this name already exists")).Show()
				return
			}

			// Получаем текущие значения
			currentAddress, _ := addressBinding.Get()
			currentPorts, _ := portsBinding.Get()
			currentPassword, _ := passwordBinding.Get()

			// Создаем нового посредника
			newRelay := Relay{
				Name:     nameEntry.Text,
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

			// Обновляем комбобокс
			updateRelaySelector()
			updateCurrentRelay(a, newRelay.Name)
			NewToast(w, "Relay saved successfully").Show()
		}, w)
	})

	// Кнопка для удаления текущего посредника
	deleteRelayBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		currentRelay := relaySelect.Selected
		if currentRelay == "" {
			return
		}

		relays, err := getRelays(a)
		if err != nil {
			NewToast(w, "Error loading relays: "+err.Error()).Show()
			return
		}

		// Не позволяем удалить последнего посредника
		if len(relays) < 2 {
			NewToast(w, lp("Cannot delete the last relay")).Show()
			return
		}

		dialog.ShowConfirm(
			lp("Delete relay"),
			lp("Are you sure you want to delete relay")+" '"+currentRelay+"'?",
			func(confirm bool) {
				if !confirm {
					return
				}

				_, index := findRelayByName(relays, currentRelay)
				if index < 0 {
					return
				}

				relays = append(relays[:index], relays[index+1:]...)
				if err := saveRelays(a, relays); err != nil {
					NewToast(w, err.Error()).Show()
					return
				}

				// Обновляем комбобокс
				updateRelaySelector()
				NewToast(w, lp("Relay deleted successfully")).Show()
			},
			w,
		)
	})

	return container.NewHBox(
		widget.NewLabelWithStyle(lp("Relay"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		relaySelect,
		layout.NewSpacer(),
		addRelayBtn,
		deleteRelayBtn,
	)
}

func createRelaySelector(a fyne.App, w fyne.Window,
	addressBinding, portsBinding, passwordBinding binding.String) (rs *fyne.Container) {

	var relaySelect *widget.Select
	var nameEntry *widget.Entry

	// Функция для обновления комбобокса из актуальных данных
	updateRelaySelector := func() {
		relays, err := getRelays(a)
		if err != nil {
			log.Tracef("Error loading relays: %v", err)
			relays = []Relay{}
		}

		// Создаем список имен посредников
		relayNames := make([]string, len(relays))
		for i, profile := range relays {
			relayNames[i] = profile.Name
		}

		// Обновляем опции комбобокса
		relaySelect.Options = relayNames

		// Устанавливаем текущего посредника
		currentRelayName := getCurrentRelayName(a)
		if currentRelayName != "" {
			if relay, index := findRelayByName(relays, currentRelayName); index >= 0 {
				relaySelect.SetSelected(relay.Name)
				updateRelayValues(relay, addressBinding, portsBinding, passwordBinding)
			} else if len(relays) > 0 {
				relaySelect.SetSelected(relays[0].Name)
				updateRelayValues(relays[0], addressBinding, portsBinding, passwordBinding)
				updateCurrentRelay(a, relays[0].Name)
			}
		} else if len(relays) > 0 {
			relaySelect.SetSelected(relays[0].Name)
			updateRelayValues(relays[0], addressBinding, portsBinding, passwordBinding)
			updateCurrentRelay(a, relays[0].Name)
		}

		relaySelect.Refresh()
	}

	// Создаем комбобокс
	relaySelect = widget.NewSelect([]string{}, func(selection string) {
		if selection != "" {
			relays, err := getRelays(a)
			if err == nil {
				if relay, index := findRelayByName(relays, selection); index >= 0 {
					updateRelayValues(relay, addressBinding, portsBinding, passwordBinding)
					updateCurrentRelay(a, selection)
				}
			}
		}
	})

	// Создаем поле ввода для имени нового посредника
	nameEntry = widget.NewEntry()
	nameEntry.SetText(" ")
	// nameEntry.SetPlaceHolder(lp("New relay name"))

	// Валидатор для проверки уникальности имени
	nameEntry.Validator = func(text string) error {
		if text == "" {
			return nil // Пустое поле - это нормально для placeholder
		}

		relays, err := getRelays(a)
		if err != nil {
			return fmt.Errorf("error loading relays")
		}

		// Проверяем, существует ли уже посредник с таким именем
		if _, index := findRelayByName(relays, text); index >= 0 {
			return fmt.Errorf(lp("A relay with this name already exists"))
		}

		return nil
	}

	// Функция добавления нового посредника
	addRelay := func() {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			NewToast(w, lp("Please enter relay name")).Show()
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
		relays, err := getRelays(a)
		if err != nil {
			NewToast(w, "Error loading relays: "+err.Error()).Show()
			return
		}

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
		updateCurrentRelay(a, name)
		nameEntry.SetText(" ")
		NewToast(w, lp("Relay added successfully")).Show()
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
			NewToast(w, lp("No relay selected")).Show()
			return
		}

		// Не даем удалить default если он последний
		if currentRelay == DEFAULT {
			relays, err := getRelays(a)
			if err == nil && len(relays) == 1 {
				NewToast(w, lp("Cannot delete the default relay")).Show()
				return
			}
		}

		// Загружаем текущие релеи
		relays, err := getRelays(a)
		if err != nil {
			NewToast(w, "Error loading relays: "+err.Error()).Show()
			return
		}

		// Находим и удаляем посредника
		_, index := findRelayByName(relays, currentRelay)
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
		NewToast(w, lp("Relay deleted successfully")).Show()
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
