package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/tcp"
	log "github.com/schollz/logger"
)

type Preferences struct {
	fyne.Preferences
	temp map[string]any
	w    fyne.Window
}

func NewPreferences(p fyne.Preferences, w fyne.Window) Preferences {
	return Preferences{
		Preferences: p,
		temp:        make(map[string]any),
		w:           w,
	}
}

func getConfigFile(requireValidPath bool, save bool) string {
	configFile, err := GetConfigDir(requireValidPath)
	if err != nil {
		log.Error(err)
		return ""
	}
	json := "receive"
	if save {
		json = "send"
	}
	return filepath.Join(configFile, json+".json")
}

func saveConfig(c Preferences, crocOptions croc.Options, save bool) {
	if c.Bool("remember") {
		configFile := getConfigFile(true, save)
		log.Debug("saving config file")
		var bConfig []byte
		// if the code wasn't set, don't save it
		if c.String("code") == "" {
			crocOptions.SharedSecret = ""
		}
		// log.Errorf("relay %s %s %v", c.String("relay"), models.DEFAULT_RELAY, c.String("relay") != models.DEFAULT_RELAY)
		if c.String("relay") != DEFAULT_RELAY {
			crocOptions.RelayAddress = "non-default: " + c.String("relay")
		} else {
			crocOptions.RelayAddress = "default"
		}
		// log.Errorf("RelayAddress %s", crocOptions.RelayAddress)

		if c.String("relay6") != DEFAULT_RELAY6 {
			crocOptions.RelayAddress6 = "non-default: " + c.String("relay6")
		} else {
			crocOptions.RelayAddress6 = "default"
		}
		bConfig, err := json.MarshalIndent(crocOptions, "", "    ")
		if err != nil {
			log.Error(err)
			return
		}
		err = os.WriteFile(configFile, bConfig, 0o644)
		if err != nil {
			log.Error(err)
			return
		}
		log.Debugf("wrote %s", configFile)
	}
}

// Вспомогательный метод для получения значения из temp
func (p Preferences) getFromTemp(key string) (any, bool) {
	if p.temp == nil {
		return nil, false
	}
	val, exists := p.temp[key]
	return val, exists
}

// Геттеры с проверкой temp
func (p Preferences) Bool(key string) bool {
	switch key {
	case "ignore-stdin", "yes", "ask", "overwrite":
		return true
	}

	if val, exists := p.getFromTemp(key); exists {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return p.Preferences.Bool(key)
}

func (p Preferences) BoolWithFallback(key string, fallback bool) bool {
	if val, exists := p.getFromTemp(key); exists {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return p.Preferences.BoolWithFallback(key, fallback)
}

func (p Preferences) Float(key string) float64 {
	if val, exists := p.getFromTemp(key); exists {
		if floatVal, ok := val.(float64); ok {
			return floatVal
		}
	}
	return p.Preferences.Float(key)
}

func (p Preferences) FloatWithFallback(key string, fallback float64) float64 {
	if val, exists := p.getFromTemp(key); exists {
		if floatVal, ok := val.(float64); ok {
			return floatVal
		}
	}
	return p.Preferences.FloatWithFallback(key, fallback)
}

func (p Preferences) Int(key string) int {
	if val, exists := p.getFromTemp(key); exists {
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}
	return p.Preferences.Int(key)
}

func (p Preferences) IntWithFallback(key string, fallback int) int {
	if val, exists := p.getFromTemp(key); exists {
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}
	return p.Preferences.IntWithFallback(key, fallback)
}

func (p Preferences) String(key string) string {
	if val, exists := p.getFromTemp(key); exists {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return p.Preferences.String(key)
}

func (p Preferences) StringWithFallback(key, fallback string) string {
	if val, exists := p.getFromTemp(key); exists {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return p.Preferences.StringWithFallback(key, fallback)
}

// Сеттеры пишут в temp
func (p Preferences) RemoveValue(key string) {
	if p.temp != nil {
		delete(p.temp, key)
	}
}

func (p Preferences) SetBool(key string, value bool) {
	if p.temp == nil {
		p.temp = make(map[string]any)
	}
	p.temp[key] = value
}

func (p Preferences) SetFloat(key string, value float64) {
	if p.temp == nil {
		p.temp = make(map[string]any)
	}
	p.temp[key] = value
}

func (p Preferences) SetInt(key string, value int) {
	if p.temp == nil {
		p.temp = make(map[string]any)
	}
	p.temp[key] = value
}

func (p Preferences) SetString(key, value string) {
	if p.temp == nil {
		p.temp = make(map[string]any)
	}
	p.temp[key] = value
}
func (p Preferences) IsSet(key string) bool {
	return true
}

func relayRun(c Preferences) (err error) {
	// log.Infof("starting croc relay version %v", Version)
	debugString := "info"
	if c.Bool("debug") {
		debugString = "debug"
	}
	host := c.String("host")
	var ports []string

	if c.IsSet("ports") {
		ports = strings.Split(c.String("ports"), ",")
	} else {
		portString := c.Int("port")
		if portString == 0 {
			portString = 9009
		}
		transfersString := c.Int("transfers")
		if transfersString == 0 {
			transfersString = 4
		}
		ports = make([]string, transfersString)
		for i := range ports {
			ports[i] = strconv.Itoa(portString + i)
		}
	}

	tcpPorts := strings.Join(ports[1:], ",")
	for i, port := range ports {
		if i == 0 {
			continue
		}
		go func(portStr string) {
			err := tcp.Run(debugString, host, portStr, determinePass(c))
			if err != nil {
				// panic(err)
				fyne.Do(func() {
					NewToast(c.w, err.Error()).Show()
				})
			}
		}(port)
	}
	return tcp.Run(debugString, host, ports[0], determinePass(c), tcpPorts)
}

func determinePass(c Preferences) (pass string) {
	pass = c.String("pass")
	b, err := os.ReadFile(pass)
	if err == nil {
		pass = strings.TrimSpace(string(b))
	}
	return
}
