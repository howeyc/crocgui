//go:build ignore

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <android/api-level.h>

// Объявляем функции ДО их использования
static char* CreateFileInDownloadsModern(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType);
static char* GetDownloadFileURI(JNIEnv* env, jobject activity, const char* fileName);

// Универсальная функция для всех версий Android
static char* CreateFileInDownloadsCompat(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType) {
    // Получаем версию Android
    int api_level = android_get_device_api_level();

    if (api_level >= 29) {
        // Android 10+ - используем MediaStore
        return CreateFileInDownloadsModern(env, activity, fileName, mimeType);
    } else {
        // Android 7-9 - возвращаем file:// URI без создания файла
        return GetDownloadFileURI(env, activity, fileName);
    }
}

// Для Android 10+ (API 29+)
static char* CreateFileInDownloadsModern(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType) {
    // 1. Get ContentResolver
    jclass activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        return strdup("error: activityClass == NULL");
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        return strdup("error: getContentResolver method not found");
    }

    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (contentResolver == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        return strdup("error: contentResolver == NULL");
    }

    // 2. Create ContentValues
    jclass contentValuesClass = (*env)->FindClass(env, "android/content/ContentValues");
    if (contentValuesClass == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        return strdup("error: ContentValues class not found");
    }

    jmethodID contentValuesInit = (*env)->GetMethodID(env, contentValuesClass, "<init>", "()V");
    if (contentValuesInit == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        return strdup("error: ContentValues constructor not found");
    }

    jobject contentValues = (*env)->NewObject(env, contentValuesClass, contentValuesInit);
    if (contentValues == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        return strdup("error: failed to create ContentValues");
    }

    // 3. Extract directory path and filename
    char* fullPath = strdup(fileName);
    char* dirPath = NULL;
    char* baseName = NULL;

    char* lastSlash = strrchr(fullPath, '/');
    if (lastSlash != NULL) {
        *lastSlash = '\0';
        dirPath = fullPath;
        baseName = lastSlash + 1;
    } else {
        baseName = fullPath;
    }

    // 4. Add values to ContentValues
    jmethodID putString = (*env)->GetMethodID(env, contentValuesClass, "put", "(Ljava/lang/String;Ljava/lang/String;)V");
    if (putString == NULL) {
        free(fullPath);
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        return strdup("error: put method not found");
    }

    // Set display name
    jstring displayNameKey = (*env)->NewStringUTF(env, "_display_name");
    jstring displayNameValue = (*env)->NewStringUTF(env, baseName);
    (*env)->CallVoidMethod(env, contentValues, putString, displayNameKey, displayNameValue);
    (*env)->DeleteLocalRef(env, displayNameKey);
    (*env)->DeleteLocalRef(env, displayNameValue);

    // Set MIME type
    jstring mimeTypeKey = (*env)->NewStringUTF(env, "mime_type");
    jstring mimeTypeValue = (*env)->NewStringUTF(env, mimeType);
    (*env)->CallVoidMethod(env, contentValues, putString, mimeTypeKey, mimeTypeValue);
    (*env)->DeleteLocalRef(env, mimeTypeKey);
    (*env)->DeleteLocalRef(env, mimeTypeValue);

    // Set relative path for Android 10+
    if (dirPath != NULL && strlen(dirPath) > 0) {
        jstring relativePathKey = (*env)->NewStringUTF(env, "relative_path");
        char relativePath[512];
        snprintf(relativePath, sizeof(relativePath), "%s/%s", "Download", dirPath);
        jstring relativePathValue = (*env)->NewStringUTF(env, relativePath);
        (*env)->CallVoidMethod(env, contentValues, putString, relativePathKey, relativePathValue);
        (*env)->DeleteLocalRef(env, relativePathKey);
        (*env)->DeleteLocalRef(env, relativePathValue);
    }

    free(fullPath);

    // 5. Get Downloads collection URI
    jclass mediaStoreClass = (*env)->FindClass(env, "android/provider/MediaStore$Downloads");
    if (mediaStoreClass == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        return strdup("error: MediaStore.Downloads class not found");
    }

    jfieldID externalContentUriField = (*env)->GetStaticFieldID(env, mediaStoreClass,
        "EXTERNAL_CONTENT_URI", "Landroid/net/Uri;");
    if (externalContentUriField == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        (*env)->DeleteLocalRef(env, mediaStoreClass);
        return strdup("error: EXTERNAL_CONTENT_URI field not found");
    }

    jobject collectionUri = (*env)->GetStaticObjectField(env, mediaStoreClass, externalContentUriField);
    if (collectionUri == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        (*env)->DeleteLocalRef(env, mediaStoreClass);
        return strdup("error: collectionUri == NULL");
    }

    // 6. Insert file into MediaStore
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        (*env)->DeleteLocalRef(env, mediaStoreClass);
        (*env)->DeleteLocalRef(env, collectionUri);
        return strdup("error: resolverClass == NULL");
    }

    jmethodID insertMethod = (*env)->GetMethodID(env, resolverClass, "insert",
        "(Landroid/net/Uri;Landroid/content/ContentValues;)Landroid/net/Uri;");
    if (insertMethod == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        (*env)->DeleteLocalRef(env, mediaStoreClass);
        (*env)->DeleteLocalRef(env, collectionUri);
        (*env)->DeleteLocalRef(env, resolverClass);
        return strdup("error: insert method not found");
    }

    jobject uri = (*env)->CallObjectMethod(env, contentResolver, insertMethod, collectionUri, contentValues);

    // Cleanup
    (*env)->DeleteLocalRef(env, activityClass);
    (*env)->DeleteLocalRef(env, contentResolver);
    (*env)->DeleteLocalRef(env, contentValuesClass);
    (*env)->DeleteLocalRef(env, contentValues);
    (*env)->DeleteLocalRef(env, mediaStoreClass);
    (*env)->DeleteLocalRef(env, collectionUri);
    (*env)->DeleteLocalRef(env, resolverClass);

    if (uri == NULL) {
        return strdup("error: MediaStore insert returned NULL");
    }

    // 7. Convert URI to string
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        (*env)->DeleteLocalRef(env, uri);
        return strdup("error: Uri class not found");
    }

    jmethodID toStringMethod = (*env)->GetMethodID(env, uriClass, "toString", "()Ljava/lang/String;");
    if (toStringMethod == NULL) {
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, uriClass);
        return strdup("error: toString method not found");
    }

    jstring uriString = (*env)->CallObjectMethod(env, uri, toStringMethod);
    if (uriString == NULL) {
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, uriClass);
        return strdup("error: uriString == NULL");
    }

    const char* utfStr = (*env)->GetStringUTFChars(env, uriString, NULL);
    char* result = strdup(utfStr);
    (*env)->ReleaseStringUTFChars(env, uriString, utfStr);

    (*env)->DeleteLocalRef(env, uriString);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, uriClass);

    return result;
}

// Для Android 7-9 (API 24-28): возвращаем file:// URI без создания файла
static char* GetDownloadFileURI(JNIEnv* env, jobject activity, const char* fileName) {
    jclass environmentClass = NULL;
    jclass fileClass = NULL;
    jstring downloadsDirType = NULL;
    jobject downloadsDirFile = NULL;
    jstring downloadsPath = NULL;

    const char* downloadsPathStr = NULL;
    char* result = NULL;

    // 1. Get Environment class
    environmentClass = (*env)->FindClass(env, "android/os/Environment");
    if (environmentClass == NULL) {
        result = strdup("error: Environment class not found");
        goto cleanup;
    }

    // 2. Get Downloads directory
    jmethodID getDownloadDirMethod = (*env)->GetStaticMethodID(env, environmentClass,
        "getExternalStoragePublicDirectory", "(Ljava/lang/String;)Ljava/io/File;");
    if (getDownloadDirMethod == NULL) {
        result = strdup("error: getExternalStoragePublicDirectory method not found");
        goto cleanup;
    }

    downloadsDirType = (*env)->NewStringUTF(env, "Download");
    downloadsDirFile = (*env)->CallStaticObjectMethod(env, environmentClass, getDownloadDirMethod, downloadsDirType);
    if (downloadsDirFile == NULL) {
        result = strdup("error: downloadsDirFile == NULL");
        goto cleanup;
    }

    // 3. Get absolute path
    fileClass = (*env)->FindClass(env, "java/io/File");
    if (fileClass == NULL) {
        result = strdup("error: File class not found");
        goto cleanup;
    }

    jmethodID getAbsolutePathMethod = (*env)->GetMethodID(env, fileClass, "getAbsolutePath", "()Ljava/lang/String;");
    if (getAbsolutePathMethod == NULL) {
        result = strdup("error: getAbsolutePath method not found");
        goto cleanup;
    }

    downloadsPath = (*env)->CallObjectMethod(env, downloadsDirFile, getAbsolutePathMethod);
    if (downloadsPath == NULL) {
        result = strdup("error: downloadsPath == NULL");
        goto cleanup;
    }

    // 4. Формируем file:// URI
    downloadsPathStr = (*env)->GetStringUTFChars(env, downloadsPath, NULL);
    if (downloadsPathStr == NULL) {
        result = strdup("error: failed to get downloads path string");
        goto cleanup;
    }

    // Вычисляем размер буфера
    size_t pathLen = strlen(downloadsPathStr);
    size_t fileNameLen = strlen(fileName);
    size_t totalLen = pathLen + fileNameLen + 10; // +10 для "file://" и "/"

    result = malloc(totalLen);
    if (result == NULL) {
        result = strdup("error: memory allocation failed");
        goto cleanup;
    }

    // Создаем file:// URI
    snprintf(result, totalLen, "file://%s/%s", downloadsPathStr, fileName);

cleanup:
    // Освобождаем ресурсы
    if (downloadsPathStr != NULL) {
        (*env)->ReleaseStringUTFChars(env, downloadsPath, downloadsPathStr);
    }

    // Удаляем LocalRef
    if (environmentClass != NULL) (*env)->DeleteLocalRef(env, environmentClass);
    if (fileClass != NULL) (*env)->DeleteLocalRef(env, fileClass);
    if (downloadsDirType != NULL) (*env)->DeleteLocalRef(env, downloadsDirType);
    if (downloadsDirFile != NULL) (*env)->DeleteLocalRef(env, downloadsDirFile);
    if (downloadsPath != NULL) (*env)->DeleteLocalRef(env, downloadsPath);

    return result;
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

// CreateFileInDownloads создает файл в папке Downloads с поддержкой всех версий Android
func CreateFileInDownloads(fileName, mimeType string) (string, error) {
	var result string
	var err error

	if mimeType == "" {
		mimeType = detectMimeType(fileName)
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		cFileName := C.CString(fileName)
		defer C.free(unsafe.Pointer(cFileName))

		cMimeType := C.CString(mimeType)
		defer C.free(unsafe.Pointer(cMimeType))

		cUri := C.CreateFileInDownloadsCompat(env, activity, cFileName, cMimeType)
		if cUri == nil {
			err = errors.New("unknown error in JNI function")
			return nil
		}

		defer C.free(unsafe.Pointer(cUri))
		resultStr := C.GoString(cUri)

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
		return "", errors.New("empty result from ContentResolver")
	}

	return result, nil
}

func ChildDownload(component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	// 1. Создаём component в Downloads
	newFileURL, err := CreateFileInDownloads(component, "")
	if err != nil {
		err = fmt.Errorf("createFileInDownloads failed: %v", err)
		return
	}

	// 2. Конвертируем в fyne.URI
	child, err = storage.ParseURI(newFileURL)
	if err != nil {
		err = fmt.Errorf("parse URI failed: %v", err)
		return
	}

	// if strings.HasPrefix(newFileURL, "file://") {
	// 	parent, err := storage.Parent(child)
	// 	if err != nil {
	// 		return nil, cleanup, fmt.Errorf("get parent failed: %v", err)
	// 	}
	// 	lu, err := storage.ListerForURI(parent)
	// 	if err != nil {
	// 		return nil, cleanup, fmt.Errorf("get lister failed: %v", err)
	// 	}
	// 	child, err = storage.Child(lu, component)
	// }

	return
}
