// webdavclient.go
package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	log "github.com/schollz/logger"
	"golang.org/x/net/webdav"
	"golang.org/x/sync/singleflight"
)

const (
	WebDAVTimeout = 3 * time.Second
)

// insecureHTTPClient — HTTP-клиент с InsecureSkipVerify для работы
// с самоподписанными сертификатами WebDAV-сервера в локальной сети
var insecureHTTPClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // самоподписанный сертификат в локальной сети
		},
	},
}

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
// InsecureSkipVerify: true — сервер использует самоподписанный сертификат,
// поэтому клиент должен пропускать проверку сертификата для локальной/LAN сети
func NewWebDAVClient() *WebDAVClient {
	return &WebDAVClient{
		httpClient: &http.Client{
			Timeout: WebDAVTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec // самоподписанный сертификат в локальной сети
				},
			},
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
	// log.Debugf("[ListChildren] Request URL: %s", u.String())

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

	// log.Debugf("[ListChildren] Response status: %d", resp.StatusCode)

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

	// log.Debugf("[ListChildren] Total response elements: %d", len(multistatus.Response))

	// Пропускаем первый элемент (саму папку) и обрабатываем детей
	var children []*WebDAVFileNode
	for i, resp := range multistatus.Response {
		if i == 0 {
			// log.Debugf("[ListChildren] Skipping root element: %v", resp.Href)
			continue // Пропускаем саму папку
		}

		if len(resp.Href) == 0 || len(resp.Propstat) == 0 {
			// log.Debugf("[ListChildren] Skipping response %d: no href or propstat", i)
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

		// log.Debugf("[ListChildren] Found child %d: name=%s, isDir=%v, path=%s", len(children), name, isDir, href)

		children = append(children, &WebDAVFileNode{
			Path:    href,
			Name:    name,
			IsDir:   isDir,
			Size:    size,
			ModTime: modTime,
		})
	}

	// log.Debugf("[ListChildren] Returning %d children for URL: %s", len(children), u.String())
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
	isRefreshing bool               // Защита от многократных обновлений
	lastRefresh  time.Time          // Время последнего обновления для debounce
	mu           sync.RWMutex       // Защита от concurrent map access
	requestGroup singleflight.Group // Предотвращает дублирование запросов
}

// NewWebDAVFileTree создает новый WebDAVFileTree
// Всегда возвращает дерево, даже при отсутствии соединения (placeholder)
func NewWebDAVFileTree(rootURL *url.URL) *WebDAVFileTree {
	// log.Debugf("[NewWebDAVFileTree] Creating tree with rootURL: %v", rootURL)

	// Если URL nil, создаём placeholder с базовым URL
	if rootURL == nil {
		log.Warnf("[NewWebDAVFileTree] rootURL is nil, creating placeholder")
		rootURL = &url.URL{
			Scheme: "http",
			Host:   "localhost:8080",
			Path:   "/",
		}
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
		client:      NewWebDAVClient(),
		rootURL:     rootURL,
		listCache:   make(map[widget.TreeNodeID][]widget.TreeNodeID),
		nodeCache:   make(map[widget.TreeNodeID]*WebDAVFileNode),
		lastRefresh: time.Now(), // Инициализируем время создания для debounce
	}

	// Добавляем корень в кэш как директорию
	rootID := rootURL.String()
	tree.nodeCache[rootID] = &WebDAVFileNode{
		Path:    rootURL.Path,
		Name:    rootURL.String(),
		IsDir:   true,
		Size:    0,
		ModTime: time.Now(),
	}
	// log.Debugf("[NewWebDAVFileTree] Added root to nodeCache as directory")

	tree.IsBranch = func(id widget.TreeNodeID) bool {
		tree.mu.RLock()
		defer tree.mu.RUnlock()
		node, ok := tree.nodeCache[id]
		return ok && node.IsDir
	}

	tree.ChildUIDs = func(id widget.TreeNodeID) []widget.TreeNodeID {
		// Если уже загружено, возвращаем из кэша
		tree.mu.RLock()
		ids, ok := tree.listCache[id]
		tree.mu.RUnlock()
		if ok {
			return ids
		}

		// Запускаем асинхронную загрузку с singleflight
		// Это предотвратит дублирование запросов к серверу
		go func() {
			ch := tree.requestGroup.DoChan(id, func() (interface{}, error) {
				return tree.loadChildren(id)
			})

			select {
			case result := <-ch:
				if result.Err != nil {
					log.Errorf("[ChildUIDs] Failed to load children for ID %s: %v", id, result.Err)
					fyne.LogError("Failed to load children for "+id, result.Err)
					// Показываем заглушку при ошибке загрузки только для корня
					if id == tree.Root {
						tree.showPlaceholder()
					}
					return
				}

				children := result.Val.([]*WebDAVFileNode)

				// Сохраняем в кэш с защитой
				var ids []widget.TreeNodeID
				for _, child := range children {
					childID := child.Path
					tree.mu.Lock()
					tree.nodeCache[childID] = child
					tree.mu.Unlock()
					ids = append(ids, childID)
				}

				tree.mu.Lock()
				tree.listCache[id] = ids
				tree.mu.Unlock()

				// Обновляем UI и открываем все ветки
				fyne.Do(func() {
					tree.Tree.Refresh()
					tree.OpenAllBranches()
				})

			case <-appCtx.Done():
				// Отмена при закрытии приложения
				return
			}
		}()

		// Возвращаем пустой список, загрузка в фоне
		return []widget.TreeNodeID{}
	}

	tree.UpdateNode = func(id widget.TreeNodeID, branch bool, node fyne.CanvasObject) {
		// log.Debugf("[UpdateNode] id=%s, branch=%v", id, branch)
		c := node.(*fyne.Container)

		var label string
		if tree.Root == id && tree.ShowRootPath {
			label = id
		} else {
			// Получаем имя из кэша или из URL
			tree.mu.RLock()
			nodeInfo, ok := tree.nodeCache[id]
			tree.mu.RUnlock()
			if ok {
				label = nodeInfo.Name
				// log.Debugf("[UpdateNode] Found in nodeCache: name=%s", label)
			} else {
				// Извлекаем из URL
				parsed, err := url.Parse(id)
				if err == nil {
					label = path.Base(parsed.Path)
					// log.Debugf("[UpdateNode] Extracted from URL path: name=%s", label)
				} else {
					label = id
					// log.Debugf("[UpdateNode] Using raw id as label: %s", label)
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
		// log.Debugf("[OnBranchClosed] Branch closed: %s", id)
		// Не удаляем кэш для корня
		if id == tree.Root {
			// log.Debugf("[OnBranchClosed] Skipping cache clear for root")
			// Вызываем Refresh принудительно
			tree.lastRefresh = time.Time{}
			tree.Refresh()
			tree.OpenAllBranches()
			return
		}
		// Очищаем кэш при закрытии папки
		tree.mu.Lock()
		delete(tree.listCache, id)
		tree.mu.Unlock()
		// log.Debugf("[OnBranchClosed] Cleared cache for: %s", id)
	}

	tree.ExtendBaseWidget(tree)
	return tree
}

// loadChildren загружает список детей для заданного узла
func (t *WebDAVFileTree) loadChildren(id widget.TreeNodeID) ([]*WebDAVFileNode, error) {
	// log.Debugf("[loadChildren] Loading children for ID: %s", id)

	// Строим полный URL для запроса
	var targetURL *url.URL
	var err error

	if id == t.Root {
		targetURL = t.rootURL
		// log.Debugf("[loadChildren] ID is root, using rootURL: %s", targetURL.String())
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
		// log.Debugf("[loadChildren] Built targetURL: %s", targetURL.String())
	}

	// Загружаем с сервера
	children, err := t.client.ListChildren(targetURL)
	if err != nil {
		log.Errorf("[loadChildren] Failed to load children from server: %v", err)
		return nil, err
	}

	// log.Debugf("[loadChildren] Loaded %d children from server", len(children))

	// Сортируем если нужно
	if t.Sorter != nil {
		// log.Debugf("[loadChildren] Using custom sorter")
		slices.SortFunc(children, func(a, b *WebDAVFileNode) int {
			if t.Sorter(a, b) {
				return -1
			}
			return 1
		})
	} else {
		// Сортировка по умолчанию: папки сначала, потом по алфавиту
		// log.Debugf("[loadChildren] Using default sorter (folders first, alphabetical)")
		slices.SortFunc(children, func(a, b *WebDAVFileNode) int {
			// Папки перед файлами
			if a.IsDir != b.IsDir {
				if a.IsDir {
					return -1
				}
				return 1
			}
			// Внутри одной группы - по алфавиту
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		})
	}

	// log.Debugf("[loadChildren] Returning %d sorted children for ID: %s", len(children), id)
	return children, nil
}

// showPlaceholder показывает заглушку CROC при ошибке соединения
func (t *WebDAVFileTree) showPlaceholder() {
	rootID := t.Root

	// Создаем новые кэши для атомарной замены
	newListCache := make(map[widget.TreeNodeID][]widget.TreeNodeID)
	newNodeCache := make(map[widget.TreeNodeID]*WebDAVFileNode)

	newNodeCache[rootID] = &WebDAVFileNode{
		Path:    t.rootURL.Path,
		Name:    CROC,
		IsDir:   true,
		Size:    0,
		ModTime: time.Now(),
	}
	newListCache[rootID] = []widget.TreeNodeID{}

	// Атомарная замена кэша
	fyne.Do(func() {
		t.mu.Lock()
		t.listCache = newListCache
		t.nodeCache = newNodeCache
		t.mu.Unlock()

		t.Tree.Refresh()
	})
}

// CheckConnection проверяет соединение с WebDAV сервером с timeout
// Возвращает nil если соединение успешно установлено
func (t *WebDAVFileTree) CheckConnection(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, WebDAVTimeout)
	defer cancel()

	// Используем клиент дерева (с InsecureSkipVerify для самоподписанных сертификатов)
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", t.rootURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Depth", "0")

	resp, err := t.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// Refresh обновляет дерево файлов почему-то на каждой вкладке
func (t *WebDAVFileTree) Refresh() {
	if at != nil {
		if atSI := at.SelectedIndex(); atSI != SENDi {
			// log.Debugf("[Refresh] from %d", atSI)
			return
		}
	}
	// Защита от многократных обновлений
	if t.isRefreshing {
		// log.Debugf("[Refresh] Already refreshing, skipping")
		return
	}
	t.isRefreshing = true
	defer func() {
		t.isRefreshing = false
	}()

	// Debounce: не обновляем если прошло менее 1s с последнего обновления
	if !t.lastRefresh.IsZero() && time.Since(t.lastRefresh) < time.Second {
		// log.Debugf("[Refresh] Skipping - refreshed too recently (%v ago)", time.Since(t.lastRefresh))
		return
	}

	t.lastRefresh = time.Now()
	// log.Debugf("[Refresh] Starting full tree refresh")

	// Запускаем асинхронное обновление с timeout и singleflight
	go func() {
		ch := t.requestGroup.DoChan("refresh", func() (interface{}, error) {
			// Проверяем соединение
			ctx, cancel := context.WithTimeout(appCtx, WebDAVTimeout)
			defer cancel()

			err := t.CheckConnection(ctx)
			if err != nil {
				return nil, fmt.Errorf("connection check failed: %w", err)
			}

			// Соединение успешно, обновляем данные
			rootID := t.Root
			rootChildren, err := t.loadChildren(rootID)
			if err != nil {
				return nil, fmt.Errorf("failed to load root children: %w", err)
			}

			// Создаем новые кэши для атомарной замены
			newListCache := make(map[widget.TreeNodeID][]widget.TreeNodeID)
			newNodeCache := make(map[widget.TreeNodeID]*WebDAVFileNode)

			newNodeCache[rootID] = &WebDAVFileNode{
				Path:    t.rootURL.Path,
				Name:    t.rootURL.String(),
				IsDir:   true,
				Size:    0,
				ModTime: time.Now(),
			}

			var rootChildIDs []widget.TreeNodeID
			for _, child := range rootChildren {
				childID := child.Path
				newNodeCache[childID] = child
				rootChildIDs = append(rootChildIDs, childID)
			}
			newListCache[rootID] = rootChildIDs

			// log.Debugf("[Refresh] Reloaded %d root children", len(rootChildIDs))

			// Возвращаем кэши для обновления UI
			return struct {
				listCache map[widget.TreeNodeID][]widget.TreeNodeID
				nodeCache map[widget.TreeNodeID]*WebDAVFileNode
			}{listCache: newListCache, nodeCache: newNodeCache}, nil
		})

		select {
		case result := <-ch:
			if result.Err != nil {
				log.Errorf("[Refresh] Failed to refresh: %v", result.Err)
				fyne.LogError("WebDAV refresh failed", result.Err)
				// Fallback на placeholder только при ошибке соединения
				if strings.Contains(result.Err.Error(), "connection check failed") {
					t.showPlaceholder()
				}
				return
			}

			caches := result.Val.(struct {
				listCache map[widget.TreeNodeID][]widget.TreeNodeID
				nodeCache map[widget.TreeNodeID]*WebDAVFileNode
			})

			// Атомарная замена кэша
			fyne.Do(func() {
				t.mu.Lock()
				t.listCache = caches.listCache
				t.nodeCache = caches.nodeCache
				t.mu.Unlock()

				// Обновляем UI виджета Tree
				t.Tree.Refresh()

				// Открываем все ветки после успешной загрузки
				t.OpenAllBranches()
			})

		case <-appCtx.Done():
			// Отмена при закрытии приложения
			return
		}
	}()
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
