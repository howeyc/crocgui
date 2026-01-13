package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// cacheEntry хранит результат и время его последнего обновления
type cacheEntry struct {
	tagName   string
	expiresAt time.Time
}

// cache - потокобезопасное хранилище кэшированных результатов
var (
	cache      = make(map[string]cacheEntry)
	cacheMutex sync.RWMutex
)

// LatestCacheTTL определяет, как долго хранить результат в кэше (например, 5 минут)
var LatestCacheTTL = 2 * time.Minute

// Latest возвращает тег последнего релиза, используя кэширование.
// owner - владелец репозитория, repo - название репозитория
func Latest(owner, repo string) (string, error) {
	// Ключ кэша: владелец+репозиторий
	cacheKey := owner + "/" + repo

	// Сначала проверяем кэш (блокировка для чтения)
	cacheMutex.RLock()
	entry, found := cache[cacheKey]
	cacheMutex.RUnlock()

	// Если запись найдена и ещё не истекла, возвращаем её
	if found && time.Now().Before(entry.expiresAt) {
		return entry.tagName, nil
	}

	// Если записи нет или она устарела, делаем реальный запрос к API
	tagName, err := fetchLatestRelease(owner, repo)
	if err != nil {
		// В случае ошибки, если у нас есть старая (но ещё актуальная) запись в кэше,
		// возвращаем её вместо ошибки (graceful degradation)
		if found && time.Now().Before(entry.expiresAt.Add(5*time.Minute)) {
			return entry.tagName, nil
		}
		return "", err
	}

	// Сохраняем новый результат в кэш (блокировка для записи)
	cacheMutex.Lock()
	cache[cacheKey] = cacheEntry{
		tagName:   tagName,
		expiresAt: time.Now().Add(LatestCacheTTL),
	}
	cacheMutex.Unlock()

	return tagName, nil
}

// fetchLatestRelease выполняет реальный HTTP-запрос к GitHub API
func fetchLatestRelease(owner, repo string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Go-App-Check-Update/1.0")
	// Опционально: добавьте токен для увеличения лимита запросов
	// req.Header.Set("Authorization", "token YOUR_GITHUB_TOKEN")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Если превышен лимит запросов, возвращаем специальную ошибку
		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			return "", fmt.Errorf("GitHub API rate limit exceeded (status %d)", resp.StatusCode)
		}
		return "", fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}

	err = json.Unmarshal(body, &release)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("release tag is empty")
	}

	return release.TagName, nil
}

// ClearCache очищает весь кэш (полезно для тестирования)
func ClearCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	cache = make(map[string]cacheEntry)
}
