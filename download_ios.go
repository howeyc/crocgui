//go:build ios

// download_ios.go
package main

/*
#include <Foundation/Foundation.h>
#include <stdlib.h>

// Получаем security-scoped bookmark для папки Downloads
char* CreateBookmarkFromURLDownload() {
    @autoreleasepool {
        // 1. Получаем URL папки Downloads
        NSFileManager *fileManager = [NSFileManager defaultManager];
        NSURL *downloadsURL = [fileManager URLForDirectory:NSDownloadsDirectory
                                                  inDomain:NSUserDomainMask
                                         appropriateForURL:nil
                                                    create:YES  // Создаем если не существует
                                                     error:nil];

        if (!downloadsURL) {
            return strdup("error: cannot get Downloads directory");
        }

        // 2. Создаем security-scoped bookmark
        NSError *error = nil;
        NSData *bookmarkData = [downloadsURL bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
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

        // 3. Конвертируем в base64 строку
        NSString *bookmarkString = [bookmarkData base64EncodedStringWithOptions:0];
        return strdup([bookmarkString UTF8String]);
    }
}

// Создает файл в указанной папке
char* CreateFileInDownloads(char* bookmarkDataStr, char* fileName, char* mimeType) {
    @autoreleasepool {
        // 1. Восстанавливаем bookmark
        NSString *bookmarkString = [NSString stringWithUTF8String:bookmarkDataStr];
        NSData *bookmarkData = [[NSData alloc] initWithBase64EncodedString:bookmarkString options:0];

        if (!bookmarkData) {
            return strdup("error: invalid bookmark data");
        }

        BOOL isStale = NO;
        NSError *error = nil;
        NSURL *downloadsURL = [NSURL URLByResolvingBookmarkData:bookmarkData
                                                        options:NSURLBookmarkResolutionWithSecurityScope
                                                  relativeToURL:nil
                                            bookmarkDataIsStale:&isStale
                                                          error:&error];

        if (error) {
            NSString *errorMsg = [NSString stringWithFormat:@"error: failed to resolve bookmark: %@", error.localizedDescription];
            return strdup([errorMsg UTF8String]);
        }

        if (isStale) {
            return strdup("error: bookmark is stale");
        }

        if (!downloadsURL) {
            return strdup("error: resolved URL is nil");
        }

        // 2. Начинаем security-scoped доступ
        if (![downloadsURL startAccessingSecurityScopedResource]) {
            return strdup("error: cannot start security-scoped access");
        }

        // 3. Создаем полный путь к файлу
        NSString *fileNameStr = [NSString stringWithUTF8String:fileName];
        NSURL *fileURL = [downloadsURL URLByAppendingPathComponent:fileNameStr];

        // 4. Создаем директории если нужно
        NSURL *directoryURL = [fileURL URLByDeletingLastPathComponent];
        if (![directoryURL isEqual:downloadsURL]) {
            // Создаем вложенные директории
            NSFileManager *fileManager = [NSFileManager defaultManager];
            [fileManager createDirectoryAtURL:directoryURL
                  withIntermediateDirectories:YES
                                   attributes:nil
                                        error:nil];
        }

        // 5. Создаем файл
        NSFileManager *fileManager = [NSFileManager defaultManager];
        if (![fileManager createFileAtPath:[fileURL path] contents:nil attributes:nil]) {
            [downloadsURL stopAccessingSecurityScopedResource];
            return strdup("error: failed to create file");
        }

        // 6. Создаем security-scoped bookmark для нового файла
        NSData *fileBookmarkData = [fileURL bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
                                     includingResourceValuesForKeys:nil
                                                      relativeToURL:nil
                                                              error:&error];

        [downloadsURL stopAccessingSecurityScopedResource];

        if (error) {
            NSString *errorMsg = [NSString stringWithFormat:@"error: failed to create file bookmark: %@", error.localizedDescription];
            return strdup([errorMsg UTF8String]);
        }

        if (!fileBookmarkData) {
            return strdup("error: file bookmark data is nil");
        }

        // 7. Конвертируем в base64 строку
        NSString *fileBookmarkString = [fileBookmarkData base64EncodedStringWithOptions:0];
        return strdup([fileBookmarkString UTF8String]);
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
	log "github.com/schollz/logger"
)

// CreateBookmarkFromURLDownload создает security-scoped bookmark для папки Downloads
func CreateBookmarkFromURLDownload() (string, error) {
	var result string
	var err error

	driver.RunNative(func(ctx interface{}) error {
		cResult := C.CreateBookmarkFromURLDownload()
		if cResult == nil {
			err = errors.New("неизвестная ошибка при создании bookmark")
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

// CreateFileInDownloads создает файл в папке Downloads
func CreateFileInDownloads(fileName, mimeType string) (string, error) {
	log.Debug("Creating file in iOS Downloads: ", fileName)

	var result string
	var err error

	// Получаем bookmark для папки Downloads
	bookmarkData, err := CreateBookmarkFromURLDownload()
	if err != nil {
		return "", fmt.Errorf("failed to get Downloads bookmark: %v", err)
	}

	driver.RunNative(func(ctx interface{}) error {
		cBookmarkData := C.CString(bookmarkData)
		defer C.free(unsafe.Pointer(cBookmarkData))

		cFileName := C.CString(fileName)
		defer C.free(unsafe.Pointer(cFileName))

		cMimeType := C.CString(mimeType)
		defer C.free(unsafe.Pointer(cMimeType))

		cResult := C.CreateFileInDownloads(cBookmarkData, cFileName, cMimeType)
		if cResult == nil {
			err = errors.New("unknown error in native function")
			return nil
		}

		defer C.free(unsafe.Pointer(cResult))
		resultStr := C.GoString(cResult)

		if strings.HasPrefix(resultStr, "error:") {
			err = errors.New(strings.TrimPrefix(resultStr, "error: "))
		} else {
			result = resultStr
		}
		return nil
	})

	if err != nil {
		log.Error("Failed to create file: ", err.Error())
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	if result == "" {
		return "", errors.New("empty result from file creation")
	}

	return result, nil
}

// ChildDownload создает файл и возвращает его для последующего наполнения данными
func ChildDownload(component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	// Создаем файл в папке Downloads
	newFileBookmark, err := CreateFileInDownloads(component, "")
	if err != nil {
		err = fmt.Errorf("CreateFileInDownloads failed: %v", err)
		return
	}

	// Разрешаем bookmark нового файла
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

	// Конвертируем в fyne.URI
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
