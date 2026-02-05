package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLsr(t *testing.T) {
	// Создаем временные директории
	baseTmp := t.TempDir()
	tmpDir := filepath.Join(baseTmp, "root")
	linkDir := filepath.Join(baseTmp, "link")

	// Создаем директории
	err := os.MkdirAll(tmpDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(linkDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем файлы в целевых директориях симлинков
	err = os.MkdirAll(filepath.Join(linkDir, "dir3"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(linkDir, "dir3", "external_file.txt"), []byte("external content"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(linkDir, "file2.txt"), []byte("external file content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем ПУСТОЙ каталог dir5 для симлинка
	err = os.MkdirAll(filepath.Join(linkDir, "dir5"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем тестовую структуру файлов в основной директории
	testStructure := []struct {
		path    string
		content string
		isDir   bool
		isLink  bool
		target  string
	}{
		{path: "file1.txt", content: "content1", isDir: false},
		{path: "dir1", isDir: true},
		{path: "dir1/subfile1.txt", content: "subcontent1", isDir: false},
		{path: "dir2", isDir: true},
		{path: "dir2/subfile2.txt", content: "subcontent2", isDir: false},
		{path: "dir3", isDir: false, isLink: true, target: filepath.Join(linkDir, "dir3")},
		{path: "file2.txt", isDir: false, isLink: true, target: filepath.Join(linkDir, "file2.txt")},
		{path: "dir4", isDir: true},                                                        // ПУСТОЙ каталог
		{path: "dir5", isDir: false, isLink: true, target: filepath.Join(linkDir, "dir5")}, // симлинк на ПУСТОЙ каталог
	}

	for _, item := range testStructure {
		fullPath := filepath.Join(tmpDir, item.path)

		if item.isDir && !item.isLink {
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				t.Fatalf("Failed to create directory %s: %v", fullPath, err)
			}
		} else if item.isLink {
			err := os.Symlink(item.target, fullPath)
			if err != nil {
				t.Fatalf("Failed to create symlink %s -> %s: %v", fullPath, item.target, err)
			}
		} else {
			err := os.WriteFile(fullPath, []byte(item.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create file %s: %v", fullPath, err)
			}
		}
	}

	// Запускаем lsr
	result, _ := lsr(tmpDir)

	// Ожидаемый результат - пустые каталоги со слешом
	expected := []string{
		"file1.txt",
		"dir1/subfile1.txt",
		"dir2/subfile2.txt",
		"dir3/external_file.txt",
		"file2.txt",
		"dir4/", // ПУСТОЙ каталог со слешом
		"dir5/", // цель симлинка на ПУСТОЙ каталог со слешом
	}

	// Проверяем результат
	if len(result) != len(expected) {
		t.Errorf("Expected %d files, got %d", len(expected), len(result))
		t.Errorf("Result: %v", result)
		t.Errorf("Expected: %v", expected)
	}

	// Проверяем наличие всех ожидаемых файлов
	for _, exp := range expected {
		found := false
		for _, res := range result {
			if res == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file not found: %s", exp)
		}
	}

	t.Logf("Test completed. Result: %v", result)
}
func TestLs(t *testing.T) {
	// Создаем временные директории
	baseTmp := t.TempDir()
	tmpDir := filepath.Join(baseTmp, "root")
	linkDir := filepath.Join(baseTmp, "link")

	// Создаем директории
	err := os.MkdirAll(tmpDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(linkDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем файлы в целевых директориях симлинков
	err = os.MkdirAll(filepath.Join(linkDir, "dir3"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(linkDir, "file2.txt"), []byte("external file content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем тестовую структуру файлов в основной директории
	testStructure := []struct {
		path    string
		content string
		isDir   bool
		isLink  bool
		target  string
	}{
		{path: "file1.txt", content: "content1", isDir: false},
		{path: "dir1", isDir: true},
		{path: "dir2", isDir: true},
		{path: "dir3", isDir: false, isLink: true, target: filepath.Join(linkDir, "dir3")},
		{path: "file2.txt", isDir: false, isLink: true, target: filepath.Join(linkDir, "file2.txt")},
	}

	for _, item := range testStructure {
		fullPath := filepath.Join(tmpDir, item.path)

		if item.isDir && !item.isLink {
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				t.Fatalf("Failed to create directory %s: %v", fullPath, err)
			}
		} else if item.isLink {
			err := os.Symlink(item.target, fullPath)
			if err != nil {
				t.Fatalf("Failed to create symlink %s -> %s: %v", fullPath, item.target, err)
			}
		} else {
			err := os.WriteFile(fullPath, []byte(item.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create file %s: %v", fullPath, err)
			}
		}
	}

	// Запускаем ls
	result := ls(tmpDir)

	// Ожидаемый результат с слешами для директорий
	expected := []string{
		"file1.txt", // обычный файл
		"dir1/",     // директория
		"dir2/",     // директория
		"dir3/",     // цель симлинка (директория)
		"file2.txt", // цель симлинка (файл)
	}

	// Проверяем результат
	if len(result) != len(expected) {
		t.Errorf("Expected %d files, got %d", len(expected), len(result))
		t.Errorf("Result: %v", result)
		t.Errorf("Expected: %v", expected)
	}

	// Проверяем наличие всех ожидаемых файлов
	for _, exp := range expected {
		found := false
		for _, res := range result {
			if res == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file not found: %s", exp)
		}
	}

	// Проверяем, что директории имеют слеш в конце
	for _, res := range result {
		if strings.HasSuffix(res, slash) {
			// Это директория - проверяем, что цель симлинка тоже директория
			if res == "dir3/" {
				// Проверяем, что dir3 действительно ведет на директорию
				dir3Path := filepath.Join(tmpDir, "dir3")
				target, err := Readlink(dir3Path)
				if err != nil {
					t.Errorf("Readlink failed for dir3: %v", err)
				}
				absTarget := target
				if !filepath.IsAbs(target) {
					absTarget = filepath.Join(filepath.Dir(dir3Path), target)
				}
				if fi, err := os.Stat(absTarget); err != nil || !fi.IsDir() {
					t.Errorf("dir3 should point to directory, but points to: %s", target)
				}
			}
		}
	}

	t.Logf("Test completed. Result: %v", result)
}
