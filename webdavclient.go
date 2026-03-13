// webdavclient.go
package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
	"golang.org/x/net/webdav"
)

// WebDAVURI представляет WebDAV URL
type WebDAVURI struct {
	*url.URL
}

// String реализует fyne.URI
func (u *WebDAVURI) String() string {
	return u.URL.String()
}

// Scheme возвращает схему URL (http или https)
func (u *WebDAVURI) Scheme() string {
	return u.URL.Scheme
}

// Authority возвращает authority часть URL (host:port)
func (u *WebDAVURI) Authority() string {
	return u.URL.Host
}

// Path возвращает путь URL
func (u *WebDAVURI) Path() string {
	return u.URL.Path
}

// Name возвращает имя ресурса (последний компонент пути)
func (u *WebDAVURI) Name() string {
	p := u.URL.Path
	if p == "/" || p == "" {
		return "/"
	}
	return path.Base(p)
}

// Parent возвращает родительский URI
func (u *WebDAVURI) Parent() fyne.URI {
	p := path.Dir(u.URL.Path)
	if p == "." || p == u.URL.Path {
		return nil
	}
	newURL := *u.URL
	newURL.Path = p
	return &WebDAVURI{URL: &newURL}
}

// Extension возвращает расширение файла
func (u *WebDAVURI) Extension() string {
	ext := path.Ext(u.URL.Path)
	if ext != "" {
		return ext[1:] // Убираем точку
	}
	return ""
}

// MimeType возвращает MIME тип (заглушка, в реальности можно определить по расширению)
func (u *WebDAVURI) MimeType() string {
	// Можно расширить для реального определения MIME типа
	return "application/octet-stream"
}

// Fragment возвращает фрагмент URL
func (u *WebDAVURI) Fragment() string {
	return u.URL.Fragment
}

// Query возвращает строку запроса URL
func (u *WebDAVURI) Query() string {
	return u.URL.RawQuery
}

// WebDAVFileNode представляет файл/каталог в WebDAV
type WebDAVFileNode struct {
	Path     string
	Name     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	ETag     string
	Children []*WebDAVFileNode
}

// WebDAVClient - HTTP клиент для работы с WebDAV сервером
type WebDAVClient struct {
	httpClient *http.Client
}

// NewWebDAVClient создает новый WebDAV клиент
func NewWebDAVClient() *WebDAVClient {
	return &WebDAVClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PropFind делает PROPFIND запрос для получения свойств ресурса
func (c *WebDAVClient) PropFind(u *url.URL, depth string) ([]webdav.Property, error) {
	// Формируем XML запрос для получения всех свойств
	propfindXML := `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
	<D:allprop/>
</D:propfind>`

	req, err := http.NewRequest("PROPFIND", u.String(), strings.NewReader(propfindXML))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", depth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Парсим XML ответ
	var multistatus struct {
		XMLName  xml.Name `xml:"DAV: multistatus"`
		Response []struct {
			Href     []string `xml:"DAV: href"`
			Propstat []struct {
				Prop []webdav.Property `xml:"DAV: prop"`
			} `xml:"DAV: propstat"`
		} `xml:"DAV: response"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&multistatus); err != nil {
		return nil, fmt.Errorf("failed to decode XML: %w", err)
	}

	if len(multistatus.Response) == 0 || len(multistatus.Response[0].Propstat) == 0 {
		return nil, fmt.Errorf("no properties in response")
	}

	return multistatus.Response[0].Propstat[0].Prop, nil
}

// ListChildren возвращает список дочерних элементов для заданного пути
func (c *WebDAVClient) ListChildren(u *url.URL) ([]*WebDAVFileNode, error) {
	log.Debugf("[ListChildren] Request URL: %s", u.String())

	// Делаем PROPFIND с Depth: 1 для получения содержимого папки
	propfindXML := `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:">
	<D:prop>
		<D:resourcetype/>
		<D:getcontentlength/>
		<D:getlastmodified/>
		<D:displayname/>
	</D:prop>
</D:propfind>`

	req, err := http.NewRequest("PROPFIND", u.String(), strings.NewReader(propfindXML))
	if err != nil {
		log.Errorf("[ListChildren] Failed to create request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Errorf("[ListChildren] Failed to execute request: %v", err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	log.Debugf("[ListChildren] Response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusMultiStatus {
		body, _ := io.ReadAll(resp.Body)
		log.Errorf("[ListChildren] Unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Парсим XML ответ
	var multistatus struct {
		XMLName  xml.Name `xml:"DAV: multistatus"`
		Response []struct {
			Href     []string `xml:"DAV: href"`
			Propstat []struct {
				Prop struct {
					XMLName       xml.Name
					ResourceType  string `xml:"DAV: resourcetype>DAV:collection"`
					ContentLength string `xml:"DAV: getcontentlength"`
					LastModified  string `xml:"DAV: getlastmodified"`
					DisplayName   string `xml:"DAV: displayname"`
					InnerXML      []byte `xml:",innerxml"`
				} `xml:"DAV: prop"`
			} `xml:"DAV: propstat"`
		} `xml:"DAV: response"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&multistatus); err != nil {
		log.Errorf("[ListChildren] Failed to decode XML: %v", err)
		return nil, fmt.Errorf("failed to decode XML: %w", err)
	}

	log.Debugf("[ListChildren] Total response elements: %d", len(multistatus.Response))

	// Пропускаем первый элемент (саму папку) и обрабатываем детей
	var children []*WebDAVFileNode
	for i, resp := range multistatus.Response {
		if i == 0 {
			log.Debugf("[ListChildren] Skipping root element: %v", resp.Href)
			continue // Пропускаем саму папку
		}

		if len(resp.Href) == 0 || len(resp.Propstat) == 0 {
			log.Debugf("[ListChildren] Skipping response %d: no href or propstat", i)
			continue
		}

		prop := resp.Propstat[0].Prop
		href := resp.Href[0]

		// Определяем тип (файл или папка) по наличию тега collection в XML
		isDir := strings.Contains(string(prop.InnerXML), "collection")

		// Парсим размер
		var size int64
		if prop.ContentLength != "" {
			fmt.Sscanf(prop.ContentLength, "%d", &size)
		}

		// Парсим дату
		var modTime time.Time
		if prop.LastModified != "" {
			modTime, _ = time.Parse(time.RFC1123, prop.LastModified)
		}

		// Определяем имя
		name := href
		if prop.DisplayName != "" {
			name = prop.DisplayName
		} else {
			// Извлекаем имя из href
			name = path.Base(href)
		}

		// Декодируем URL (может содержать %20 и т.д.)
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}

		log.Debugf("[ListChildren] Found child %d: name=%s, isDir=%v, path=%s", len(children), name, isDir, href)

		children = append(children, &WebDAVFileNode{
			Path:    href,
			Name:    name,
			IsDir:   isDir,
			Size:    size,
			ModTime: modTime,
		})
	}

	log.Debugf("[ListChildren] Returning %d children for URL: %s", len(children), u.String())
	return children, nil
}

// WebDAVFileTree - виджет дерева файлов WebDAV
type WebDAVFileTree struct {
	widget.Tree
	ShowRootPath bool
	Sorter       func(*WebDAVFileNode, *WebDAVFileNode) bool

	client       *WebDAVClient
	rootURL      *url.URL
	listCache    map[widget.TreeNodeID][]widget.TreeNodeID
	nodeCache    map[widget.TreeNodeID]*WebDAVFileNode
	loadingNodes map[widget.TreeNodeID]bool
	isRefreshing bool      // Защита от многократных обновлений
	lastRefresh  time.Time // Время последнего обновления для debounce
}

// NewWebDAVFileTree создает новый WebDAVFileTree
// Возвращает nil если не удается подключиться к серверу
func NewWebDAVFileTree(rootURL *url.URL) *WebDAVFileTree {
	log.Debugf("[NewWebDAVFileTree] Creating tree with rootURL: %s", rootURL.String())

	// Проверяем что URL валиден
	if rootURL == nil {
		log.Errorf("[NewWebDAVFileTree] rootURL is nil")
		return nil
	}

	tree := &WebDAVFileTree{
		Tree: widget.Tree{
			Root: rootURL.String(),
			CreateNode: func(branch bool) fyne.CanvasObject {
				var icon fyne.CanvasObject
				if branch {
					icon = widget.NewIcon(nil)
				} else {
					icon = widget.NewFileIcon(nil)
				}
				return container.NewBorder(nil, nil, icon, nil, widget.NewLabel("Template Object"))
			},
		},
		client:       NewWebDAVClient(),
		rootURL:      rootURL,
		listCache:    make(map[widget.TreeNodeID][]widget.TreeNodeID),
		nodeCache:    make(map[widget.TreeNodeID]*WebDAVFileNode),
		loadingNodes: make(map[widget.TreeNodeID]bool),
		lastRefresh:  time.Now(), // Инициализируем время создания для debounce
	}

	// Проверяем соединение с сервером
	log.Debugf("[NewWebDAVFileTree] Testing connection to server...")
	rootChildren, err := tree.loadChildren(rootURL.String())
	if err != nil {
		log.Errorf("[NewWebDAVFileTree] Failed to connect to WebDAV server: %v", err)
		fyne.LogError("Failed to connect to WebDAV server", err)
		return nil
	}

	log.Debugf("[NewWebDAVFileTree] Connection successful! Root has %d children", len(rootChildren))

	// Сохраняем детей корня в кэш для отображения
	rootID := rootURL.String()

	// Добавляем сам корень в кэш как директорию
	tree.nodeCache[rootID] = &WebDAVFileNode{
		Path:    rootURL.Path,
		Name:    rootURL.String(),
		IsDir:   true,
		Size:    0,
		ModTime: time.Now(),
	}
	log.Debugf("[NewWebDAVFileTree] Added root to nodeCache as directory")

	var rootChildIDs []widget.TreeNodeID
	for _, child := range rootChildren {
		childID := child.Path
		tree.nodeCache[childID] = child
		rootChildIDs = append(rootChildIDs, childID)
	}
	tree.listCache[rootID] = rootChildIDs
	log.Debugf("[NewWebDAVFileTree] Cached %d root children", len(rootChildIDs))

	tree.IsBranch = func(id widget.TreeNodeID) bool {
		node, ok := tree.nodeCache[id]
		return ok && node.IsDir
	}

	tree.ChildUIDs = func(id widget.TreeNodeID) []widget.TreeNodeID {
		// Если уже загружено, возвращаем из кэша
		if ids, ok := tree.listCache[id]; ok {
			return ids
		}

		// Загружаем с сервера
		children, err := tree.loadChildren(id)
		if err != nil {
			log.Errorf("[ChildUIDs] Failed to load children for ID %s: %v", id, err)
			fyne.LogError("Failed to load children for "+id, err)
			return nil
		}

		// Сохраняем в кэш
		var ids []widget.TreeNodeID
		for _, child := range children {
			childID := child.Path
			tree.nodeCache[childID] = child
			ids = append(ids, childID)
		}

		tree.listCache[id] = ids
		return ids
	}

	tree.UpdateNode = func(id widget.TreeNodeID, branch bool, node fyne.CanvasObject) {
		log.Debugf("[UpdateNode] id=%s, branch=%v", id, branch)
		c := node.(*fyne.Container)

		var label string
		if tree.Root == id && tree.ShowRootPath {
			label = id
		} else {
			// Получаем имя из кэша или из URL
			if nodeInfo, ok := tree.nodeCache[id]; ok {
				label = nodeInfo.Name
				log.Debugf("[UpdateNode] Found in nodeCache: name=%s", label)
			} else {
				// Извлекаем из URL
				parsed, err := url.Parse(id)
				if err == nil {
					label = path.Base(parsed.Path)
					log.Debugf("[UpdateNode] Extracted from URL path: name=%s", label)
				} else {
					label = id
					log.Debugf("[UpdateNode] Using raw id as label: %s", label)
				}
			}
		}

		c.Objects[0].(*widget.Label).SetText(label)

		if branch {
			var r fyne.Resource
			if tree.IsBranchOpen(id) {
				r = theme.FolderOpenIcon()
			} else {
				r = theme.FolderIcon()
			}
			c.Objects[1].(*widget.Icon).SetResource(r)
		} else {
			// Для файлов используем FileIcon
			if fileIcon, ok := c.Objects[1].(*widget.FileIcon); ok {
				fileIcon.SetURI(&WebDAVURI{URL: &url.URL{
					Scheme: tree.rootURL.Scheme,
					Host:   tree.rootURL.Host,
					Path:   id,
				}})
			}
		}
	}

	tree.OnBranchClosed = func(id widget.TreeNodeID) {
		log.Debugf("[OnBranchClosed] Branch closed: %s", id)
		// Не удаляем кэш для корня
		if id == tree.Root {
			log.Debugf("[OnBranchClosed] Skipping cache clear for root")
			return
		}
		// Очищаем кэш при закрытии папки
		delete(tree.listCache, id)
		log.Debugf("[OnBranchClosed] Cleared cache for: %s", id)
	}

	tree.ExtendBaseWidget(tree)
	return tree
}

// loadChildren загружает список детей для заданного узла
func (t *WebDAVFileTree) loadChildren(id widget.TreeNodeID) ([]*WebDAVFileNode, error) {
	log.Debugf("[loadChildren] Loading children for ID: %s", id)

	// Строим полный URL для запроса
	var targetURL *url.URL
	var err error

	if id == t.Root {
		targetURL = t.rootURL
		log.Debugf("[loadChildren] ID is root, using rootURL: %s", targetURL.String())
	} else {
		targetURL, err = url.Parse(id)
		if err != nil {
			log.Errorf("[loadChildren] Failed to parse ID %s: %v", id, err)
			return nil, err
		}
		// Восстанавливаем полную ссылку
		if targetURL.Scheme == "" {
			targetURL.Scheme = t.rootURL.Scheme
		}
		if targetURL.Host == "" {
			targetURL.Host = t.rootURL.Host
		}
		log.Debugf("[loadChildren] Built targetURL: %s", targetURL.String())
	}

	// Загружаем с сервера
	children, err := t.client.ListChildren(targetURL)
	if err != nil {
		log.Errorf("[loadChildren] Failed to load children from server: %v", err)
		return nil, err
	}

	log.Debugf("[loadChildren] Loaded %d children from server", len(children))

	// Сортируем если нужно
	if t.Sorter != nil {
		log.Debugf("[loadChildren] Using custom sorter")
		for i := 0; i < len(children); i++ {
			for j := i + 1; j < len(children); j++ {
				if !t.Sorter(children[i], children[j]) {
					children[i], children[j] = children[j], children[i]
				}
			}
		}
	} else {
		// Сортировка по умолчанию: папки сначала, потом по алфавиту
		log.Debugf("[loadChildren] Using default sorter (folders first, alphabetical)")
		for i := 0; i < len(children); i++ {
			for j := i + 1; j < len(children); j++ {
				// Папки перед файлами
				if children[i].IsDir != children[j].IsDir {
					if !children[i].IsDir {
						children[i], children[j] = children[j], children[i]
					}
				} else {
					// Внутри одной группы - по алфавиту
					if children[i].Name > children[j].Name {
						children[i], children[j] = children[j], children[i]
					}
				}
			}
		}
	}

	log.Debugf("[loadChildren] Returning %d sorted children for ID: %s", len(children), id)
	return children, nil
}

// Refresh обновляет дерево файлов
func (t *WebDAVFileTree) Refresh() {
	// Защита от многократных обновлений
	if t.isRefreshing {
		log.Debugf("[Refresh] Already refreshing, skipping")
		return
	}
	t.isRefreshing = true
	defer func() {
		t.isRefreshing = false
	}()

	// Debounce: не обновляем если прошло менее 500ms с последнего обновления
	if !t.lastRefresh.IsZero() && time.Since(t.lastRefresh) < time.Second {
		log.Debugf("[Refresh] Skipping - refreshed too recently (%v ago)", time.Since(t.lastRefresh))
		return
	}

	t.lastRefresh = time.Now()
	log.Debugf("[Refresh] Starting full tree refresh")

	// 1. Очищаем весь кэш
	t.listCache = make(map[widget.TreeNodeID][]widget.TreeNodeID)
	t.nodeCache = make(map[widget.TreeNodeID]*WebDAVFileNode)
	log.Debugf("[Refresh] Cleared all caches")

	// 2. Перезагружаем корневые элементы с сервера
	rootID := t.Root
	rootChildren, err := t.loadChildren(rootID)
	if err != nil {
		log.Errorf("[Refresh] Failed to reload root children: %v", err)
		fyne.LogError("Failed to reload root children", err)
		return
	}

	// 3. Сохраняем корень и его детей в новый кэш
	t.nodeCache[rootID] = &WebDAVFileNode{
		Path:    t.rootURL.Path,
		Name:    t.rootURL.String(),
		IsDir:   true,
		Size:    0,
		ModTime: time.Now(),
	}

	var rootChildIDs []widget.TreeNodeID
	for _, child := range rootChildren {
		childID := child.Path
		t.nodeCache[childID] = child
		rootChildIDs = append(rootChildIDs, childID)
	}
	t.listCache[rootID] = rootChildIDs

	log.Debugf("[Refresh] Reloaded %d root children", len(rootChildIDs))

	// 4. Обновляем UI виджета Tree
	t.Tree.Refresh()

	log.Debugf("[Refresh] Tree refresh completed")
}

// GetNodeURL возвращает URL для заданного узла
func (t *WebDAVFileTree) GetNodeURL(id widget.TreeNodeID) (*url.URL, error) {
	if id == t.Root {
		return t.rootURL, nil
	}

	parsed, err := url.Parse(id)
	if err != nil {
		return nil, err
	}

	// Восстанавливаем полную ссылку
	if parsed.Scheme == "" {
		parsed.Scheme = t.rootURL.Scheme
	}
	if parsed.Host == "" {
		parsed.Host = t.rootURL.Host
	}

	return parsed, nil
}
