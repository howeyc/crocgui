//go:build ios

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

func ChildDownload(component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	// 1. Получаем bookmark для папки Downloads
	bookmarkData, err := CreateBookmarkFromURLDownload()
	if err != nil {
		err = fmt.Errorf("CreateBookmarkFromURLDownload failed: %v", err)
		return
	}

	// 2. Разрешаем bookmark для получения parent URI
	parentURL, isStale, err := ResolveBookmarkToURL(bookmarkData)
	if err != nil {
		err = fmt.Errorf("resolve parent bookmark failed: %v", err)
		return
	}
	defer StopAccessingSecurityScopedResource(parentURL)

	if isStale {
		err = fmt.Errorf("parent bookmark is stale")
		return
	}

	// 3. Конвертируем в fyne.URI для проверок
	parentURI, err := storage.ParseURI(parentURL)
	if err != nil {
		err = fmt.Errorf("parse parent URI failed: %v", err)
		return
	}

	// 4. Проверяем, что parent является директорией
	canList, err := storage.CanList(parentURI)
	if err != nil {
		err = fmt.Errorf("cannot check if listable: %v", err)
		return
	}
	if !canList {
		err = fmt.Errorf("parent is not a directory: %s", parentURI.String())
		return
	}

	// 5. Создаем файл в папке Downloads
	newFileURL, err := CreateFileInTree(bookmarkData, component, "")
	if err != nil {
		err = fmt.Errorf("CreateFileInTree failed: %v", err)
		return
	}

	// 6. Создаем security-scoped bookmark для нового файла
	newFileBookmark, err := CreateBookmarkFromURL(newFileURL)
	if err != nil {
		err = fmt.Errorf("create bookmark for new file failed: %v", err)
		return
	}

	// 7. Разрешаем bookmark нового файла
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

	// 8. Конвертируем в fyne.URI
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
