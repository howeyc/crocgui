package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/models"
	"github.com/schollz/croc/v10/src/utils"
	log "github.com/schollz/logger"
)

type Preferences struct {
	fyne.Preferences
	paths  []string
	second bool
	temp   map[string]any
}

func NewPreferences(p fyne.Preferences, paths ...string) Preferences {
	return Preferences{
		Preferences: p,
		paths:       paths,
		temp:        make(map[string]any), // инициализируем мапу
	}
}

type Slicer interface {
	Slice() []string
}

type Paths []string

func (p Paths) Slice() []string {
	return p
}

func (c *Preferences) Args() Slicer {
	return Paths(c.paths)
}

func (c *Preferences) IsSet(key string) bool {
	return false
}

type Client struct {
	options                croc.Options
	filesInfo              []croc.FileInfo
	emptyFoldersToTransfer []croc.FileInfo
	totalNumberFolders     int
}

var spy Client

func (c *Client) Send(filesInfo []croc.FileInfo, emptyFoldersToTransfer []croc.FileInfo, totalNumberFolders int) (err error) {
	spy.filesInfo = filesInfo
	spy.emptyFoldersToTransfer = emptyFoldersToTransfer
	spy.totalNumberFolders = totalNumberFolders
	return nil
}

func getSendConfigFile(requireValidPath bool) string {
	configFile, err := utils.GetConfigDir(requireValidPath)
	if err != nil {
		log.Error(err)
		return ""
	}
	return path.Join(configFile, "send.json")
}

func getClassicConfigFile(requireValidPath bool) string {
	configFile, err := utils.GetConfigDir(requireValidPath)
	if err != nil {
		log.Error(err)
		return ""
	}
	return path.Join(configFile, "classic_enabled")
}

func getReceiveConfigFile(requireValidPath bool) (string, error) {
	configFile, err := utils.GetConfigDir(requireValidPath)
	if err != nil {
		log.Error(err)
		return "", err
	}
	return path.Join(configFile, "receive.json"), nil
}

func determinePass(c Preferences) (pass string) {
	pass = c.String("pass")
	b, err := os.ReadFile(pass)
	if err == nil {
		pass = strings.TrimSpace(string(b))
	}
	return
}

func saveConfig(c Preferences, crocOptions croc.Options) {
	if c.Bool("remember") {
		configFile := getSendConfigFile(true)
		log.Debug("saving config file")
		var bConfig []byte
		// if the code wasn't set, don't save it
		if c.String("code") == "" {
			crocOptions.SharedSecret = ""
		}
		// log.Errorf("relay %s %s %v", c.String("relay"), models.DEFAULT_RELAY, c.String("relay") != models.DEFAULT_RELAY)
		if c.String("relay") != models.DEFAULT_RELAY {
			crocOptions.RelayAddress = "non-default: " + c.String("relay")
		} else {
			crocOptions.RelayAddress = "default"
		}
		// log.Errorf("RelayAddress %s", crocOptions.RelayAddress)

		if c.String("relay6") != models.DEFAULT_RELAY6 {
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

func setDebugLevel(c Preferences) {
	if c.Bool("quiet") {
		log.SetLevel("error")
	} else if c.Bool("debug") {
		log.SetLevel("debug")
		log.Debug("debug mode on")
		// print the public IP address
		ip, err := utils.PublicIP()
		if err == nil {
			log.Debugf("public IP address: %s", ip)
		} else {
			log.Debug(err)
		}

	} else {
		log.SetLevel("info")
	}
}
func makeTempFileWithString(s string) (fnames []string, err error) {
	f, err := os.CreateTemp(".", "croc-stdin-")
	if err != nil {
		return
	}

	_, err = f.WriteString(s)
	if err != nil {
		return
	}

	err = f.Close()
	if err != nil {
		return
	}
	fnames = []string{f.Name()}
	return
}

func send(c Preferences) (err error) {
	setDebugLevel(c)
	comm.Socks5Proxy = c.String("socks5")
	comm.HttpProxy = c.String("connect")

	portParam := c.Int("port")
	if portParam == 0 {
		portParam = 9009
	}
	transfersParam := c.Int("transfers")
	if transfersParam == 0 {
		transfersParam = 4
	}
	excludeStrings := []string{}
	for _, v := range strings.Split(c.String("exclude"), ",") {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			excludeStrings = append(excludeStrings, v)
		}
	}

	ports := make([]string, transfersParam+1)
	for i := 0; i <= transfersParam; i++ {
		ports[i] = strconv.Itoa(portParam + i)
	}

	crocOptions := croc.Options{
		SharedSecret:      c.String("code"),
		IsSender:          true,
		Debug:             c.Bool("debug"),
		NoPrompt:          c.Bool("yes"),
		RelayAddress:      c.String("relay"),
		RelayAddress6:     c.String("relay6"),
		Stdout:            c.Bool("stdout"),
		DisableLocal:      c.Bool("no-local"),
		OnlyLocal:         c.Bool("local"),
		IgnoreStdin:       c.Bool("ignore-stdin"),
		RelayPorts:        ports,
		Ask:               c.Bool("ask"),
		NoMultiplexing:    c.Bool("no-multi"),
		RelayPassword:     determinePass(c),
		SendingText:       c.String("text") != "",
		NoCompress:        c.Bool("no-compress"),
		Overwrite:         c.Bool("overwrite"),
		Curve:             c.String("curve"),
		HashAlgorithm:     c.String("hash"),
		ThrottleUpload:    c.String("throttleUpload"),
		ZipFolder:         c.Bool("zip"),
		GitIgnore:         c.Bool("git"),
		ShowQrCode:        c.Bool("qrcode"),
		MulticastAddress:  c.String("multicast"),
		Exclude:           excludeStrings,
		Quiet:             c.Bool("quiet"),
		DisableClipboard:  c.Bool("disable-clipboard"),
		ExtendedClipboard: c.Bool("extended-clipboard"),
	}
	if crocOptions.RelayAddress != models.DEFAULT_RELAY {
		crocOptions.RelayAddress6 = ""
	} else if crocOptions.RelayAddress6 != models.DEFAULT_RELAY6 {
		crocOptions.RelayAddress = ""
	}

	log.Tracef("crocOptions %+v", crocOptions)

	b, errOpen := os.ReadFile(getSendConfigFile(false))
	if errOpen == nil && !c.Bool("remember") {

		var rememberedOptions croc.Options
		err = json.Unmarshal(b, &rememberedOptions)
		if err != nil {
			log.Error(err)
			return
		}
		log.Tracef("rememberedOptions %+v", rememberedOptions)
		// update anything that isn't explicitly set
		if !c.IsSet("no-local") {
			crocOptions.DisableLocal = rememberedOptions.DisableLocal
		}
		if !c.IsSet("ports") && len(rememberedOptions.RelayPorts) > 0 {
			crocOptions.RelayPorts = rememberedOptions.RelayPorts
		}
		if !c.IsSet("code") {
			crocOptions.SharedSecret = rememberedOptions.SharedSecret
		}
		if !c.IsSet("pass") && rememberedOptions.RelayPassword != "" {
			crocOptions.RelayPassword = rememberedOptions.RelayPassword
		}
		if !c.IsSet("overwrite") {
			crocOptions.Overwrite = rememberedOptions.Overwrite
		}
		if !c.IsSet("curve") && rememberedOptions.Curve != "" {
			crocOptions.Curve = rememberedOptions.Curve
		}
		if !c.IsSet("local") {
			crocOptions.OnlyLocal = rememberedOptions.OnlyLocal
		}
		if !c.IsSet("hash") {
			crocOptions.HashAlgorithm = rememberedOptions.HashAlgorithm
		}
		if !c.IsSet("git") {
			crocOptions.GitIgnore = rememberedOptions.GitIgnore
		}
		if !c.IsSet("relay") && strings.HasPrefix(rememberedOptions.RelayAddress, "non-default:") {
			var rememberedAddr = strings.TrimPrefix(rememberedOptions.RelayAddress, "non-default:")
			rememberedAddr = strings.TrimSpace(rememberedAddr)
			crocOptions.RelayAddress = rememberedAddr
		}
		if !c.IsSet("relay6") && strings.HasPrefix(rememberedOptions.RelayAddress6, "non-default:") {
			var rememberedAddr = strings.TrimPrefix(rememberedOptions.RelayAddress6, "non-default:")
			rememberedAddr = strings.TrimSpace(rememberedAddr)
			crocOptions.RelayAddress6 = rememberedAddr
		}
		log.Tracef("rememberedOptions %+v", rememberedOptions)
	}

	var fnames []string
	stat, _ := os.Stdin.Stat()
	if ((stat.Mode() & os.ModeCharDevice) == 0) && !c.Bool("ignore-stdin") {
		fnames, err = getStdin()
		if err != nil {
			return
		}
		utils.MarkFileForRemoval(fnames[0])
		defer func() {
			e := os.Remove(fnames[0])
			if e != nil {
				log.Error(e)
			}
		}()
	} else if c.String("text") != "" {
		fnames, err = makeTempFileWithString(c.String("text"))
		if err != nil {
			return
		}
		utils.MarkFileForRemoval(fnames[0])
		defer func() {
			e := os.Remove(fnames[0])
			if e != nil {
				log.Error(e)
			}
		}()

	} else {
		fnames = c.Args().Slice()
	}
	if len(fnames) == 0 {
		return errors.New("must specify file: croc send [filename(s) or folder]")
	}

	classicInsecureMode := utils.Exists(getClassicConfigFile(true))
	if !classicInsecureMode {
		// if operating system is UNIX, then use environmental variable to set the code
		if (!(runtime.GOOS == "windows") && c.IsSet("code")) || os.Getenv("CROC_SECRET") != "" {
			crocOptions.SharedSecret = os.Getenv("CROC_SECRET")
			if crocOptions.SharedSecret == "" {
				fmt.Printf(`On UNIX systems, to send with a custom code phrase,
you need to set the environmental variable CROC_SECRET:

  CROC_SECRET=**** croc send file.txt

Or you can have the code phrase automatically generated:

  croc send file.txt

Or you can go back to the classic croc behavior by enabling classic mode:

  croc --classic

`)
				os.Exit(0)
			}
		}
	}

	if len(crocOptions.SharedSecret) == 0 {
		// generate code phrase
		crocOptions.SharedSecret = utils.GetRandomName()
	}
	minimalFileInfos, emptyFoldersToTransfer, totalNumberFolders, err := croc.GetFilesInfo(fnames, crocOptions.ZipFolder, crocOptions.GitIgnore, crocOptions.Exclude)
	log.Tracef("GetFilesInfo %v", err)
	if err != nil {
		return
	}
	if len(crocOptions.Exclude) > 0 {
		minimalFileInfosInclude := []croc.FileInfo{}
		emptyFoldersToTransferInclude := []croc.FileInfo{}
		for _, f := range minimalFileInfos {
			exclude := false
			for _, exclusion := range crocOptions.Exclude {
				if strings.Contains(path.Join(strings.ToLower(f.FolderRemote), strings.ToLower(f.Name)), exclusion) {
					exclude = true
					break
				}
			}
			if !exclude {
				minimalFileInfosInclude = append(minimalFileInfosInclude, f)
			}
		}
		for _, f := range emptyFoldersToTransfer {
			exclude := false
			for _, exclusion := range crocOptions.Exclude {
				if strings.Contains(path.Join(strings.ToLower(f.FolderRemote), strings.ToLower(f.Name)), exclusion) {
					exclude = true
					break
				}
			}
			if !exclude {
				emptyFoldersToTransferInclude = append(emptyFoldersToTransferInclude, f)
			}
		}
		totalNumberFolders = 0
		folderMap := make(map[string]bool)
		for _, f := range minimalFileInfosInclude {
			folderMap[f.FolderRemote] = true
		}
		for _, f := range emptyFoldersToTransferInclude {
			folderMap[f.FolderRemote] = true
		}
		totalNumberFolders = len(folderMap)
		minimalFileInfos = minimalFileInfosInclude
		emptyFoldersToTransfer = emptyFoldersToTransferInclude
	}

	cr, err := crocNew(crocOptions)
	if err != nil {
		return
	}

	// save the config
	saveConfig(c, crocOptions)
	err = cr.Send(minimalFileInfos, emptyFoldersToTransfer, totalNumberFolders)
	return
}

func crocNew(options croc.Options) (c *Client, err error) {
	spy.options = options
	return nil, nil
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

// Дополнительные методы для работы с temp
func (p Preferences) ClearTemp() {
	p.temp = make(map[string]any)
}

func (p Preferences) GetTemp() map[string]any {
	return p.temp
}

func (p Preferences) ApplyTemp() {
	for key, value := range p.temp {
		switch v := value.(type) {
		case bool:
			p.Preferences.SetBool(key, v)
		case float64:
			p.Preferences.SetFloat(key, v)
		case int:
			p.Preferences.SetInt(key, v)
		case string:
			p.Preferences.SetString(key, v)
		}
	}
	p.ClearTemp()
}
