// http.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
)

func (h *WebDAVWithDirectoryListing) serveDirectoryListing(w http.ResponseWriter, r *http.Request) {
	// Открываем директорию через FileSystem
	f, err := h.fileSystem.OpenFile(context.Background(), r.URL.Path, os.O_RDONLY, 0)
	if err != nil {
		http.Error(w, "Error opening directory", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Проверяем, что это действительно директория через Stat
	stat, err := f.Stat()
	if err != nil || !stat.IsDir() {
		http.Error(w, "Not a directory", http.StatusBadRequest)
		return
	}

	// Для webdav.File нужно использовать Readdir если он поддерживается
	type readdirFile interface {
		Readdir(count int) ([]os.FileInfo, error)
	}

	if readdir, ok := f.(readdirFile); ok {
		// Читаем все файлы в директории
		fileInfos, err := readdir.Readdir(-1)
		if err != nil {
			http.Error(w, "Error reading directory", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Нормализуем путь для отображения
		displayPath := r.URL.Path
		if displayPath == "" {
			displayPath = "/"
		}
		displayPath = "/" + strings.TrimLeft(path.Clean(displayPath), "/")
		if displayPath == "/." {
			displayPath = "/"
		}

		// Формируем кликабельную цепочку родительских каталогов
		breadcrumbs := h.generateBreadcrumbs(displayPath)

		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<title>Index of %s</title>
	<link rel="icon" type="image/png" href="/favicon.ico">
    <link rel="shortcut icon" href="/favicon.ico" type="image/x-icon">
    <link rel="apple-touch-icon" href="/favicon.ico">
	<style>
		body { 
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
			margin: 20px;
			background-color: #f5f5f5;
		}
		.container {
			max-width: 1200px;
			margin: 0 auto;
			background-color: white;
			border-radius: 8px;
			box-shadow: 0 2px 4px rgba(0,0,0,0.1);
			padding: 20px;
		}
		.breadcrumbs {
			margin: 0 0 20px 0;
			padding: 10px;
			background-color: #f8f9fa;
			border-radius: 4px;
			font-size: 1.1em;
		}
		.breadcrumbs a {
			color: #0066cc;
			text-decoration: none;
		}
		.breadcrumbs a:hover {
			text-decoration: underline;
		}
		.breadcrumbs .separator {
			color: #666;
			margin: 0 5px;
		}
		.breadcrumbs .current {
			color: #333;
			font-weight: 500;
		}
		.directory-listing {
			width: 100%%;
			border-collapse: collapse;
		}
		.directory-listing tr:hover {
			background-color: #f5f5f5;
		}
		.directory-listing td {
			padding: 8px 12px;
			border-bottom: 1px solid #eee;
		}
		.directory-listing .name {
			width: 60%%;
		}
		.directory-listing .size {
			width: 15%%;
			text-align: right;
			color: #666;
			font-family: monospace;
		}
		.directory-listing .date {
			width: 25%%;
			color: #666;
			font-family: monospace;
		}
		.directory-listing a {
			color: #0066cc;
			text-decoration: none;
		}
		.directory-listing a:hover {
			text-decoration: underline;
		}
		.footer {
			margin-top: 20px;
			text-align: right;
			color: #666;
			font-size: 0.9em;
			border-top: 1px solid #eee;
			padding-top: 10px;
		}
	</style>
</head>
<body>
	<div class="container">
		%s
		<table class="directory-listing">
			<tbody>
`, displayPath, breadcrumbs)

		// Проверяем каждый элемент через Stat FileSystem для правильного определения типа
		for _, info := range fileInfos {
			name := info.Name()

			// Получаем полный путь в FileSystem
			fullPath := path.Join(r.URL.Path, name)

			// Используем Stat от FileSystem для определения типа
			// Это гарантирует правильную обработку симлинков через ResolvingFileSystem
			stat, err := h.fileSystem.Stat(context.Background(), fullPath)
			if err != nil {
				// Если не можем получить stat, пропускаем элемент
				continue
			}

			// Формируем путь для ссылки - без суффикса /
			filePath := path.Join(r.URL.Path, name)
			if stat.IsDir() {
				name += "/"
			}

			// Форматируем размер (только для файлов)
			var size string
			if !stat.IsDir() {
				size = formatSize(stat.Size())
			}

			// Форматируем дату
			modTime := stat.ModTime().Format("2006-01-02 15:04:05")

			fmt.Fprintf(w, `<tr>
	<td class="name"><a href="%s">%s</a></td>
	<td class="size">%s</td>
	<td class="date">%s</td>
</tr>
`, filePath, name, size, modTime)
		}

		fmt.Fprintf(w, `			</tbody>
		</table>
		<div class="footer">
			%d items
		</div>
	</div>
</body>
</html>`, len(fileInfos))
	} else {
		// Если не можем получить интерфейс с Readdir, используем стандартный WebDAV handler
		h.webdavHandler.ServeHTTP(w, r)
	}
}

// formatSize форматирует размер файла в человеко-читаемый вид
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// generateBreadcrumbs создает кликабельную цепочку родительских каталогов
func (h *WebDAVWithDirectoryListing) generateBreadcrumbs(currentPath string) string {
	if currentPath == "/" {
		return `<div class="breadcrumbs"><a href="/">root</a></div>`
	}

	parts := strings.Split(strings.Trim(currentPath, "/"), "/")
	var breadcrumbs strings.Builder
	breadcrumbs.WriteString(`<div class="breadcrumbs"><a href="/">root</a>`)

	pathSoFar := ""
	for i, part := range parts {
		pathSoFar += "/" + part
		separator := `<span class="separator">›</span>`
		if i == len(parts)-1 {
			// Текущий каталог (не ссылка)
			breadcrumbs.WriteString(fmt.Sprintf(`%s <span class="current">%s</span>`, separator, part))
		} else {
			// Родительский каталог (ссылка)
			breadcrumbs.WriteString(fmt.Sprintf(`%s <a href="%s">%s</a>`, separator, pathSoFar, part))
		}
	}
	breadcrumbs.WriteString("</div>")
	return breadcrumbs.String()
}
