//go:build ios

package main

/*
#include <Foundation/Foundation.h>
#include <UIKit/UIKit.h>
#include <stdlib.h>

// Создание security-scoped bookmark из URL
char* CreateBookmarkFromURL(const char* urlString) {
    @autoreleasepool {
        NSString *nsUrlString = [NSString stringWithUTF8String:urlString];
        NSURL *url = [NSURL URLWithString:nsUrlString];

        if (!url) {
            return strdup("error: invalid URL");
        }

        NSError *error = nil;
        NSData *bookmarkData = [url bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
                             includingResourceValuesForKeys:nil
                                              relativeToURL:nil
                                                      error:&error];

        if (error) {
            NSString *errorMsg = [NSString stringWithFormat:@"error: failed to create bookmark: %@", error.localizedDescription];
            return strdup([errorMsg UTF8String]);
        }

        if (!bookmarkData) {
            return strdup("error: bookmark data is nil");
        }

        NSString *bookmarkString = [bookmarkData base64EncodedStringWithOptions:0];
        return strdup([bookmarkString UTF8String]);
    }
}

// Разрешение bookmark'а в URL с security scope
char* ResolveBookmarkToURL(const char* bookmarkDataString, bool* isStaleOut) {
    @autoreleasepool {
        NSString *nsBookmarkData = [NSString stringWithUTF8String:bookmarkDataString];
        NSData *bookmarkData = [[NSData alloc] initWithBase64EncodedString:nsBookmarkData options:0];

        if (!bookmarkData) {
            return strdup("error: invalid bookmark data");
        }

        NSError *error = nil;
        BOOL isStale = NO;
        NSURL *url = [NSURL URLByResolvingBookmarkData:bookmarkData
                                               options:NSURLBookmarkResolutionWithSecurityScope
                                         relativeToURL:nil
                                   bookmarkDataIsStale:&isStale
                                                 error:&error];

        if (error) {
            NSString *errorMsg = [NSString stringWithFormat:@"error: failed to resolve bookmark: %@", error.localizedDescription];
            return strdup([errorMsg UTF8String]);
        }

        if (!url) {
            return strdup("error: resolved URL is nil");
        }

        if (isStaleOut) {
            *isStaleOut = isStale;
        }

        // Начинаем security-scoped доступ
        if (![url startAccessingSecurityScopedResource]) {
            return strdup("error: failed to start security scoped access");
        }

        return strdup([url.absoluteString UTF8String]);
    }
}

// Остановка security-scoped доступа
void StopAccessingSecurityScopedResource(const char* urlString) {
    @autoreleasepool {
        NSString *nsUrlString = [NSString stringWithUTF8String:urlString];
        NSURL *url = [NSURL URLWithString:nsUrlString];

        if (url) {
            [url stopAccessingSecurityScopedResource];
        }
    }
}

// Функция для создания файла через security-scoped bookmark
char* CreateFileInTreeIOS(const char* bookmarkData, const char* fileName, const char* mimeType) {
    @autoreleasepool {
        NSString *nsBookmarkData = [NSString stringWithUTF8String:bookmarkData];
        NSString *nsFileName = [NSString stringWithUTF8String:fileName];
        NSString *nsMimeType = [NSString stringWithUTF8String:mimeType];

        // Разрешаем bookmark в URL
        NSData *bookmarkDataObj = [[NSData alloc] initWithBase64EncodedString:nsBookmarkData options:0];
        if (!bookmarkDataObj) {
            return strdup("error: invalid bookmark data");
        }

        NSError *error = nil;
        BOOL isStale = NO;
        NSURL *targetURL = [NSURL URLByResolvingBookmarkData:bookmarkDataObj
                                                    options:NSURLBookmarkResolutionWithSecurityScope
                                              relativeToURL:nil
                                        bookmarkDataIsStale:&isStale
                                                      error:&error];

        if (error || !targetURL) {
            NSString *errorMsg = [NSString stringWithFormat:@"error: failed to resolve bookmark: %@", error ? error.localizedDescription : @"unknown error"];
            return strdup([errorMsg UTF8String]);
        }

        // Начинаем security-scoped доступ
        if (![targetURL startAccessingSecurityScopedResource]) {
            return strdup("error: failed to access security scoped resource");
        }

        // Проверяем, что это директория
        NSNumber *isDirectory = nil;
        if (![targetURL getResourceValue:&isDirectory forKey:NSURLIsDirectoryKey error:&error] || !isDirectory.boolValue) {
            [targetURL stopAccessingSecurityScopedResource];
            return strdup("error: target is not a directory");
        }

        // Создаем файл внутри директории
        NSURL *newFileURL = [targetURL URLByAppendingPathComponent:nsFileName];
        NSData *emptyData = [NSData data];
        BOOL success = [emptyData writeToURL:newFileURL options:NSDataWritingAtomic error:&error];

        // Останавливаем доступ
        [targetURL stopAccessingSecurityScopedResource];

        if (!success) {
            NSString *errorMsg = [NSString stringWithFormat:@"error: failed to create file: %@", error.localizedDescription];
            return strdup([errorMsg UTF8String]);
        }

        // Создаем bookmark для нового файла
        NSData *newBookmarkData = [newFileURL bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
                                       includingResourceValuesForKeys:nil
                                                        relativeToURL:nil
                                                                error:&error];

        if (error) {
            // Все равно возвращаем URL, даже если не удалось создать bookmark
            return strdup([newFileURL.absoluteString UTF8String]);
        }

        // Возвращаем URL нового файла
        return strdup([newFileURL.absoluteString UTF8String]);
    }
}
*/
import "C"
import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/storage"
)

// CreateBookmarkFromURL создает security-scoped bookmark из URL
func CreateBookmarkFromURL(url string) (string, error) {
	var result string
	var err error

	driver.RunNative(func(ctx interface{}) error {
		cUrl := C.CString(url)
		defer C.free(unsafe.Pointer(cUrl))

		cResult := C.CreateBookmarkFromURL(cUrl)
		if cResult == nil {
			err = errors.New("CreateBookmarkFromURL is nil")
			return nil
		}

		defer C.free(unsafe.Pointer(cResult))
		resultStr := C.GoString(cResult)

		if strings.HasPrefix(resultStr, "error:") {
			err = errors.New(resultStr)
		} else {
			result = resultStr
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return result, nil
}

// ResolveBookmarkToURL разрешает security-scoped bookmark в URL
func ResolveBookmarkToURL(bookmarkData string) (string, bool, error) {
	var result string
	var isStale bool
	var err error

	driver.RunNative(func(ctx interface{}) error {
		cBookmarkData := C.CString(bookmarkData)
		defer C.free(unsafe.Pointer(cBookmarkData))

		var cIsStale C.bool
		cResult := C.ResolveBookmarkToURL(cBookmarkData, &cIsStale)
		if cResult == nil {
			err = errors.New("bookmark is nil")
			return nil
		}

		defer C.free(unsafe.Pointer(cResult))
		resultStr := C.GoString(cResult)
		isStale = bool(cIsStale)

		if strings.HasPrefix(resultStr, "error:") {
			err = errors.New(resultStr)
		} else {
			result = resultStr
		}
		return nil
	})

	if err != nil {
		return "", false, err
	}
	return result, isStale, nil
}

// StopAccessingSecurityScopedResource останавливает security-scoped доступ
func StopAccessingSecurityScopedResource(url string) {
	driver.RunNative(func(ctx interface{}) error {
		cUrl := C.CString(url)
		defer C.free(unsafe.Pointer(cUrl))

		C.StopAccessingSecurityScopedResource(cUrl)
		return nil
	})
}

// CreateFileInTree создает файл в указанной через bookmark директории на iOS
func CreateFileInTree(bookmarkData, fileName, mimeType string) (string, error) {
	var result string
	var err error

	if mimeType == "" {
		mimeType = detectMimeType(fileName)
	}

	driver.RunNative(func(ctx interface{}) error {
		cBookmarkData := C.CString(bookmarkData)
		defer C.free(unsafe.Pointer(cBookmarkData))
		cFileName := C.CString(fileName)
		defer C.free(unsafe.Pointer(cFileName))
		cMimeType := C.CString(mimeType)
		defer C.free(unsafe.Pointer(cMimeType))

		cResult := C.CreateFileInTreeIOS(cBookmarkData, cFileName, cMimeType)
		if cResult == nil {
			err = errors.New("createFileInTreeIOS is nil")
			return nil
		}

		defer C.free(unsafe.Pointer(cResult))
		resultStr := C.GoString(cResult)

		if strings.HasPrefix(resultStr, "error:") {
			err = errors.New(resultStr)
		} else {
			result = resultStr
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if result == "" {
		return "", errors.New("result is empty")
	}

	return result, nil
}

func Child(parent fyne.URI, component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	// 1. Пробуем стандартный способ
	child, err = storage.Child(parent, component)
	if err == nil {
		return
	}

	// 2. Проверяем, что parent является директорией
	canList, err := storage.CanList(parent)
	if err != nil {
		err = fmt.Errorf("cannot check if listable: %v", err)
		return
	}
	if !canList {
		err = fmt.Errorf("URI is not a directory: %s", parent.String())
		return
	}

	// 3. iOS-specific логика
	bookmarkData, err := CreateBookmarkFromURL(parent.String())
	if err != nil {
		err = fmt.Errorf("createBookmarkFromURL failed: %v", err)
		return
	}

	// 4. Создаём component в parent
	newFileURL, err := CreateFileInTree(bookmarkData, component, "")
	if err != nil {
		err = fmt.Errorf("CreateFileInTree failed: %v", err)
		return
	}

	// 5. Создаем security-scoped доступ
	newFileBookmark, err := CreateBookmarkFromURL(newFileURL)
	if err != nil {
		err = fmt.Errorf("create bookmark for new file failed: %v", err)
		return
	}

	// 6. Конвертируем в URL
	resolvedURL, isStale, err := ResolveBookmarkToURL(newFileBookmark)
	if err != nil {
		err = fmt.Errorf("resolveBookmarkToURL failed: %v", err)
		return
	}

	if isStale {
		StopAccessingSecurityScopedResource(resolvedURL)
		err = fmt.Errorf("bookmark is stale for %s", resolvedURL)
		return
	}

	// 7. Конвертируем в fyne.URI
	child, err = storage.ParseURI(resolvedURL)
	if err != nil {
		StopAccessingSecurityScopedResource(resolvedURL)
		err = fmt.Errorf("parse URI failed: %v", err)
		return
	}

	cleanup = func() {
		StopAccessingSecurityScopedResource(resolvedURL)
	}

	return
}
