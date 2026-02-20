//go:build android

// media_android.go
package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

// Структура для передачи параметров в JNI
typedef struct {
    const char* collection_type;  // "images", "video", "audio", "downloads"
    const char* relative_path;    // относительный путь
    const char* file_name;        // имя файла
    const char* mime_type;        // MIME-тип
} MediaStoreParams;

// Функция для создания файла через MediaStore
char* CreateFileViaMediaStore(JNIEnv* env, jobject activity, MediaStoreParams* params) {
    if (!params || !params->file_name) {
        return strdup("error: invalid parameters");
    }

    char* result = NULL;
    jobject content_resolver = NULL;
    jclass activity_class = NULL;
    jobject content_values = NULL;
    jobject collection_uri = NULL;

    // 1. Получаем ContentResolver
    activity_class = (*env)->GetObjectClass(env, activity);
    if (!activity_class) goto cleanup;

    jmethodID get_content_resolver = (*env)->GetMethodID(env, activity_class, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (!get_content_resolver) goto cleanup;

    content_resolver = (*env)->CallObjectMethod(env, activity, get_content_resolver);
    if (!content_resolver) goto cleanup;

    // 2. Получаем URI коллекции
    const char* collection_class = NULL;
    const char* collection_field = "EXTERNAL_CONTENT_URI";

    if (strcmp(params->collection_type, "images") == 0) {
        collection_class = "android/provider/MediaStore$Images$Media";
    } else if (strcmp(params->collection_type, "video") == 0) {
        collection_class = "android/provider/MediaStore$Video$Media";
    } else if (strcmp(params->collection_type, "audio") == 0) {
        collection_class = "android/provider/MediaStore$Audio$Media";
    } else {
        collection_class = "android/provider/MediaStore$Downloads";
    }

    jclass media_class = (*env)->FindClass(env, collection_class);
    if (!media_class) goto cleanup;

    jfieldID uri_field = (*env)->GetStaticFieldID(env, media_class, collection_field, "Landroid/net/Uri;");
    if (!uri_field) goto cleanup;

    collection_uri = (*env)->GetStaticObjectField(env, media_class, uri_field);
    if (!collection_uri) goto cleanup;

    // 3. Создаем ContentValues
    jclass content_values_class = (*env)->FindClass(env, "android/content/ContentValues");
    if (!content_values_class) goto cleanup;

    jmethodID values_init = (*env)->GetMethodID(env, content_values_class, "<init>", "()V");
    jmethodID put_string = (*env)->GetMethodID(env, content_values_class, "put", "(Ljava/lang/String;Ljava/lang/String;)V");

    content_values = (*env)->NewObject(env, content_values_class, values_init);

    // Устанавливаем поля
    jstring name_key = (*env)->NewStringUTF(env, "_display_name");
    jstring name_value = (*env)->NewStringUTF(env, params->file_name);
    (*env)->CallVoidMethod(env, content_values, put_string, name_key, name_value);

    jstring mime_key = (*env)->NewStringUTF(env, "mime_type");
    jstring mime_value = (*env)->NewStringUTF(env, params->mime_type);
    (*env)->CallVoidMethod(env, content_values, put_string, mime_key, mime_value);

    // Устанавливаем RELATIVE_PATH если есть
    if (params->relative_path && strlen(params->relative_path) > 0) {
        jstring path_key = (*env)->NewStringUTF(env, "relative_path");
        jstring path_value = (*env)->NewStringUTF(env, params->relative_path);
        (*env)->CallVoidMethod(env, content_values, put_string, path_key, path_value);
    }

    // 4. Вставляем запись
    jclass resolver_class = (*env)->GetObjectClass(env, content_resolver);
    jmethodID insert_method = (*env)->GetMethodID(env, resolver_class, "insert", "(Landroid/net/Uri;Landroid/content/ContentValues;)Landroid/net/Uri;");

    jobject new_file_uri = (*env)->CallObjectMethod(env, content_resolver, insert_method, collection_uri, content_values);

    if (new_file_uri) {
        jclass uri_class = (*env)->FindClass(env, "android/net/Uri");
        jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
        jstring uri_str = (*env)->CallObjectMethod(env, new_file_uri, to_string);

        const char* uri_cstr = (*env)->GetStringUTFChars(env, uri_str, NULL);
        result = strdup(uri_cstr);
        (*env)->ReleaseStringUTFChars(env, uri_str, uri_cstr);
    } else {
        result = strdup("error: MediaStore insert returned null");
    }

cleanup:
    if (activity_class) (*env)->DeleteLocalRef(env, activity_class);
    if (media_class) (*env)->DeleteLocalRef(env, media_class);
    if (content_values_class) (*env)->DeleteLocalRef(env, content_values_class);
    if (resolver_class) (*env)->DeleteLocalRef(env, resolver_class);
    if (collection_uri) (*env)->DeleteLocalRef(env, collection_uri);
    if (content_values) (*env)->DeleteLocalRef(env, content_values);
    if (content_resolver) (*env)->DeleteLocalRef(env, content_resolver);

    if (!result) {
        result = strdup("error: failed to create file via MediaStore");
    }

    return result;
}
*/
import "C"

import (
	"net/url"
	"strings"
)

// MediaStorePathInfo содержит информацию для создания файла через MediaStore
type MediaStorePathInfo struct {
	CollectionType string // "images", "video", "audio", "downloads", "files"
	RelativePath   string // относительный путь внутри коллекции
	FileName       string // имя файла
	MimeType       string // MIME-тип
}

// ParseUriForMediaStore парсит URI и определяет параметры для MediaStore
func ParseUriForMediaStore(uri string) (*MediaStorePathInfo, error) {
	info := &MediaStorePathInfo{
		CollectionType: "downloads", // значение по умолчанию
	}

	// Декодируем URL-encoded URI
	decodedURI, err := url.QueryUnescape(uri)
	if err != nil {
		decodedURI = uri // используем как есть если декодирование не удалось
	}

	// Анализируем тип URI
	switch {
	case strings.Contains(decodedURI, "content://media/"):
		// MediaStore URI
		if strings.Contains(decodedURI, "/images/media") {
			info.CollectionType = "images"
		} else if strings.Contains(decodedURI, "/video/media") {
			info.CollectionType = "video"
		} else if strings.Contains(decodedURI, "/audio/media") {
			info.CollectionType = "audio"
		} else if strings.Contains(decodedURI, "/downloads") {
			info.CollectionType = "downloads"
		}

	case strings.Contains(decodedURI, "content://com.android.externalstorage.documents/document/primary"):
		// DocumentsContract URI - извлекаем путь из primary
		path := extractPathFromPrimaryURI(decodedURI)
		info.RelativePath, info.CollectionType = analyzeStoragePath(path)

	case strings.Contains(decodedURI, "content://com.android.providers.downloads.documents/document/raw%3A"):
		// Raw storage path from downloads provider
		path := extractRawPathFromDownloadsURI(decodedURI)
		info.RelativePath, info.CollectionType = analyzeStoragePath(path)

	case strings.HasPrefix(decodedURI, "file://"):
		// File URI - извлекаем путь и анализируем
		path := strings.TrimPrefix(decodedURI, "file://")
		info.RelativePath, info.CollectionType = analyzeStoragePath(path)

	default:
		// Неизвестный URI тип, используем downloads по умолчанию
		info.CollectionType = "downloads"
	}

	return info, nil
}

// extractPathFromPrimaryURI извлекает путь из external storage URI
func extractPathFromPrimaryURI(uri string) string {
	// content://com.android.externalstorage.documents/document/primary%3ADownload%2FMyFolder
	primaryPrefix := "primary%3A"
	primaryIndex := strings.Index(uri, primaryPrefix)
	if primaryIndex == -1 {
		return ""
	}

	path := uri[primaryIndex+len(primaryPrefix):]

	// Удаляем возможные параметры после ?
	if questionIndex := strings.Index(path, "?"); questionIndex != -1 {
		path = path[:questionIndex]
	}

	return path
}

// extractRawPathFromDownloadsURI извлекает путь из downloads provider URI
func extractRawPathFromDownloadsURI(uri string) string {
	// content://com.android.providers.downloads.documents/document/raw%3A%2Fstorage%2Femulated%2F0%2FDownload
	rawPrefix := "raw%3A"
	rawIndex := strings.Index(uri, rawPrefix)
	if rawIndex == -1 {
		return ""
	}

	path := uri[rawIndex+len(rawPrefix):]

	// Декодируем дополнительно если нужно
	decoded, err := url.QueryUnescape(path)
	if err == nil {
		path = decoded
	}

	return path
}

// analyzeStoragePath анализирует путь в хранилище и определяет коллекцию
func analyzeStoragePath(fullPath string) (relativePath, collectionType string) {
	// Убираем начальный слеш если есть
	path := strings.TrimPrefix(fullPath, "/")

	// Определяем коллекцию на основе известных папок
	switch {
	case strings.HasPrefix(path, "DCIM/") || strings.HasPrefix(path, "Pictures/"):
		collectionType = "images"
		relativePath = extractRelativePath(path, "Pictures/", "DCIM/")

	case strings.HasPrefix(path, "Movies/"):
		collectionType = "video"
		relativePath = extractRelativePath(path, "Movies/")

	case strings.HasPrefix(path, "Music/") || strings.HasPrefix(path, "Alarms/") ||
		strings.HasPrefix(path, "Podcasts/") || strings.HasPrefix(path, "Ringtones/"):
		collectionType = "audio"
		relativePath = extractRelativePath(path, "Music/", "Alarms/", "Podcasts/", "Ringtones/")

	case strings.HasPrefix(path, "Download/"):
		collectionType = "downloads"
		relativePath = extractRelativePath(path, "Download/")

	case strings.HasPrefix(path, "Documents/"):
		collectionType = "files"
		relativePath = extractRelativePath(path, "Documents/")

	default:
		// Для произвольных путей используем downloads
		collectionType = "downloads"
		relativePath = path
	}

	return relativePath, collectionType
}

// extractRelativePath извлекает относительный путь после базовых папок
func extractRelativePath(fullPath string, baseFolders ...string) string {
	for _, folder := range baseFolders {
		if strings.HasPrefix(fullPath, folder) {
			relative := strings.TrimPrefix(fullPath, folder)

			// Убираем имя файла из пути (последний компонент)
			if lastSlash := strings.LastIndex(relative, "/"); lastSlash != -1 {
				return relative[:lastSlash]
			}
			return "" // нет подпапок, только файл в корне
		}
	}
	return fullPath
}

// PrepareMediaStoreParams подготавливает параметры для создания файла через MediaStore
func PrepareMediaStoreParams(uri, fileName string) (*MediaStorePathInfo, error) {
	info, err := ParseUriForMediaStore(uri)
	if err != nil {
		return nil, err
	}

	info.FileName = fileName
	info.MimeType = detectMimeType(fileName)

	return info, nil
}
