//go:build android

// download_android.go

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <errno.h>
#include <fcntl.h>
#include <unistd.h>
#include <sys/time.h>

#include <sys/types.h>
#include <utime.h>
#include <time.h>

void LogD(const char* message);

// Функция для проверки разрешения
static jboolean hasPermission(JNIEnv* env, jobject context, const char* permission) {
    jclass context_class = (*env)->GetObjectClass(env, context);
    if (context_class == NULL) {
        LogD("C: ERROR - Failed to get context class in hasPermission");
        return JNI_FALSE;
    }

    jmethodID check_permission = (*env)->GetMethodID(env, context_class, "checkSelfPermission", "(Ljava/lang/String;)I");
    if (check_permission == NULL) {
        LogD("C: ERROR - Failed to get checkSelfPermission method");
        (*env)->DeleteLocalRef(env, context_class);
        return JNI_FALSE;
    }

    jstring permission_str = (*env)->NewStringUTF(env, permission);
    jint result = (*env)->CallIntMethod(env, context, check_permission, permission_str);

    (*env)->DeleteLocalRef(env, permission_str);
    (*env)->DeleteLocalRef(env, context_class);

    jboolean has_perm = (result == 0) ? JNI_TRUE : JNI_FALSE;

    char permLog[128];
    snprintf(permLog, sizeof(permLog), "C: Permission %s: %s", permission, (has_perm == JNI_TRUE) ? "GRANTED" : "DENIED");
    LogD(permLog);

    return has_perm; // 0 = PERMISSION_GRANTED
}

// Функция для запроса разрешений
static void requestStoragePermissions(JNIEnv* env, jobject activity, const char** permissions, jint size) {
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class in requestStoragePermissions");
        return;
    }

    jmethodID request_permissions = (*env)->GetMethodID(env, activity_class, "requestPermissions", "([Ljava/lang/String;I)V");
    if (request_permissions == NULL) {
        LogD("C: ERROR - Failed to get requestPermissions method");
        (*env)->DeleteLocalRef(env, activity_class);
        return;
    }

    // Создаем массив строк Java
    jclass string_class = (*env)->FindClass(env, "java/lang/String");
    jobjectArray permissions_array = (*env)->NewObjectArray(env, size, string_class, NULL);

    for (int i = 0; i < size; i++) {
        jstring permission = (*env)->NewStringUTF(env, permissions[i]);
        (*env)->SetObjectArrayElement(env, permissions_array, i, permission);
        (*env)->DeleteLocalRef(env, permission);
    }

    // Вызываем requestPermissions
    (*env)->CallVoidMethod(env, activity, request_permissions, permissions_array, 123); // 123 - request code

    // Очищаем ресурсы
    (*env)->DeleteLocalRef(env, permissions_array);
    (*env)->DeleteLocalRef(env, string_class);
    (*env)->DeleteLocalRef(env, activity_class);

    LogD("C: Storage permissions requested");
}

// Проверка и запрос необходимых разрешений для Android < 10
static jboolean checkAndRequestStoragePermissions(JNIEnv* env, jobject activity) {
    const char* permissions[] = {
        "android.permission.READ_EXTERNAL_STORAGE",
        "android.permission.WRITE_EXTERNAL_STORAGE"
    };
    jint size = sizeof(permissions) / sizeof(permissions[0]);

    jboolean allGranted = JNI_TRUE;

    // Проверяем текущие разрешения
    for (int i = 0; i < size; i++) {
        if (!hasPermission(env, activity, permissions[i])) {
            allGranted = JNI_FALSE;
            break;
        }
    }

    // Если не все разрешения есть - запрашиваем
    if (!allGranted) {
        LogD("C: Requesting storage permissions");
        requestStoragePermissions(env, activity, permissions, size);
        return JNI_FALSE;
    }

    LogD("C: All storage permissions granted");
    return JNI_TRUE;
}

// Функция для получения API уровня
static jint get_api_level(JNIEnv* env) {
    jclass version_class = (*env)->FindClass(env, "android/os/Build$VERSION");
    if (version_class == NULL) {
        return -1;
    }

    jfieldID sdk_int_field = (*env)->GetStaticFieldID(env, version_class, "SDK_INT", "I");
    if (sdk_int_field == NULL) {
        (*env)->DeleteLocalRef(env, version_class);
        return -1;
    }

    jint sdk_int = (*env)->GetStaticIntField(env, version_class, sdk_int_field);
    (*env)->DeleteLocalRef(env, version_class);

    return sdk_int;
}

// Функция для создания папок в MediaStore (Android 10+)
static jboolean createDirectoriesInMediaStore(JNIEnv* env, jobject contentResolver, const char* relativePath) {
    if (relativePath == NULL || strlen(relativePath) == 0) {
        return JNI_TRUE;
    }

    char pathLog[512];
    snprintf(pathLog, sizeof(pathLog), "C: Creating directories for path: %s", relativePath);
    LogD(pathLog);

    // Разбиваем путь на компоненты
    char pathCopy[512];
    strncpy(pathCopy, relativePath, sizeof(pathCopy) - 1);
    pathCopy[sizeof(pathCopy) - 1] = '\0';

    char* token = strtok(pathCopy, "/");
    char currentPath[512] = "";
    jboolean success = JNI_TRUE;

    while (token != NULL && success == JNI_TRUE) {
        // Обновляем текущий путь
        if (strlen(currentPath) > 0) {
            strcat(currentPath, "/");
        }
        strcat(currentPath, token);

        char dirLog[256];
        snprintf(dirLog, sizeof(dirLog), "C: Creating directory: %s (token: %s)", currentPath, token);
        LogD(dirLog);

        // Создаем ContentValues для папки
        jclass contentValuesClass = (*env)->FindClass(env, "android/content/ContentValues");
        if (contentValuesClass == NULL) {
            LogD("C: ERROR - ContentValues class not found");
            success = JNI_FALSE;
            break;
        }

        jmethodID contentValuesInit = (*env)->GetMethodID(env, contentValuesClass, "<init>", "()V");
        jmethodID putString = (*env)->GetMethodID(env, contentValuesClass, "put", "(Ljava/lang/String;Ljava/lang/String;)V");

        if (contentValuesInit == NULL || putString == NULL) {
            LogD("C: ERROR - ContentValues methods not found");
            (*env)->DeleteLocalRef(env, contentValuesClass);
            success = JNI_FALSE;
            break;
        }

        jobject contentValues = (*env)->NewObject(env, contentValuesClass, contentValuesInit);
        if (contentValues == NULL) {
            LogD("C: ERROR - Failed to create ContentValues");
            (*env)->DeleteLocalRef(env, contentValuesClass);
            success = JNI_FALSE;
            break;
        }

        // Устанавливаем относительный путь
        jstring relativePathKey = (*env)->NewStringUTF(env, "relative_path");
        jstring relativePathValue = (*env)->NewStringUTF(env, "Download"); // Базовая папка Downloads
        (*env)->CallVoidMethod(env, contentValues, putString, relativePathKey, relativePathValue);
        (*env)->DeleteLocalRef(env, relativePathKey);
        (*env)->DeleteLocalRef(env, relativePathValue);

        // Устанавливаем имя папки
        jstring displayNameKey = (*env)->NewStringUTF(env, "_display_name");
        jstring displayNameValue = (*env)->NewStringUTF(env, currentPath);
        (*env)->CallVoidMethod(env, contentValues, putString, displayNameKey, displayNameValue);
        (*env)->DeleteLocalRef(env, displayNameKey);
        (*env)->DeleteLocalRef(env, displayNameValue);

        // Устанавливаем MIME-тип для папки
        jstring mimeTypeKey = (*env)->NewStringUTF(env, "mime_type");
        jstring mimeTypeValue = (*env)->NewStringUTF(env, "vnd.android.document/directory");
        (*env)->CallVoidMethod(env, contentValues, putString, mimeTypeKey, mimeTypeValue);
        (*env)->DeleteLocalRef(env, mimeTypeKey);
        (*env)->DeleteLocalRef(env, mimeTypeValue);

        // Вставляем папку в MediaStore
        jclass mediaStoreClass = (*env)->FindClass(env, "android/provider/MediaStore$Downloads");
        if (mediaStoreClass == NULL) {
            LogD("C: ERROR - MediaStore.Downloads class not found");
            (*env)->DeleteLocalRef(env, contentValuesClass);
            (*env)->DeleteLocalRef(env, contentValues);
            success = JNI_FALSE;
            break;
        }

        jfieldID externalContentUriField = (*env)->GetStaticFieldID(env, mediaStoreClass, "EXTERNAL_CONTENT_URI", "Landroid/net/Uri;");
        if (externalContentUriField == NULL) {
            LogD("C: ERROR - EXTERNAL_CONTENT_URI field not found");
            (*env)->DeleteLocalRef(env, contentValuesClass);
            (*env)->DeleteLocalRef(env, contentValues);
            (*env)->DeleteLocalRef(env, mediaStoreClass);
            success = JNI_FALSE;
            break;
        }

        jobject collectionUri = (*env)->GetStaticObjectField(env, mediaStoreClass, externalContentUriField);
        if (collectionUri == NULL) {
            LogD("C: ERROR - collectionUri is NULL");
            (*env)->DeleteLocalRef(env, contentValuesClass);
            (*env)->DeleteLocalRef(env, contentValues);
            (*env)->DeleteLocalRef(env, mediaStoreClass);
            success = JNI_FALSE;
            break;
        }

        jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
        if (resolverClass == NULL) {
            LogD("C: ERROR - resolverClass is NULL");
            (*env)->DeleteLocalRef(env, contentValuesClass);
            (*env)->DeleteLocalRef(env, contentValues);
            (*env)->DeleteLocalRef(env, mediaStoreClass);
            (*env)->DeleteLocalRef(env, collectionUri);
            success = JNI_FALSE;
            break;
        }

        jmethodID insertMethod = (*env)->GetMethodID(env, resolverClass, "insert", "(Landroid/net/Uri;Landroid/content/ContentValues;)Landroid/net/Uri;");
        if (insertMethod == NULL) {
            LogD("C: ERROR - insert method not found");
            (*env)->DeleteLocalRef(env, contentValuesClass);
            (*env)->DeleteLocalRef(env, contentValues);
            (*env)->DeleteLocalRef(env, mediaStoreClass);
            (*env)->DeleteLocalRef(env, collectionUri);
            (*env)->DeleteLocalRef(env, resolverClass);
            success = JNI_FALSE;
            break;
        }

        // Пытаемся создать папку (игнорируем ошибки если папка уже существует)
        jobject folderUri = (*env)->CallObjectMethod(env, contentResolver, insertMethod, collectionUri, contentValues);

        if (folderUri != NULL) {
            LogD("C: Directory created successfully");
            (*env)->DeleteLocalRef(env, folderUri);
        } else {
            LogD("C: Directory may already exist or creation failed (ignoring)");
            // Не считаем это ошибкой - папка может уже существовать
        }

        // Очистка ресурсов для этой итерации
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        (*env)->DeleteLocalRef(env, mediaStoreClass);
        (*env)->DeleteLocalRef(env, collectionUri);
        (*env)->DeleteLocalRef(env, resolverClass);

        token = strtok(NULL, "/");
    }

    if (success == JNI_TRUE) {
        LogD("C: All directories created successfully");
    } else {
        LogD("C: Directory creation failed");
    }

    return success;
}

// Для Android 10+ (API 29+) с возвратом только URI
static char* CreateFileInDownloadsModern(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType) {
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

    char* fullPath = strdup(fileName);
    char* dirPath = NULL;
    char* baseName = NULL;

    char* lastSlash = strrchr(fullPath, '/');
    if (lastSlash != NULL) {
        *lastSlash = '\0';
        dirPath = fullPath;
        baseName = lastSlash + 1;

        // Если есть путь с директориями - создаем их
        LogD("C: Path contains directories, creating them first");
        if (!createDirectoriesInMediaStore(env, contentResolver, dirPath)) {
            LogD("C: WARNING: Directory creation may have failed, continuing anyway");
        }
    } else {
        baseName = fullPath;
        LogD("C: Path does not contain directories");
    }

    char debugLog[512];
    snprintf(debugLog, sizeof(debugLog), "C: Creating file - dirPath: '%s', baseName: '%s'",
             dirPath ? dirPath : "NULL", baseName);
    LogD(debugLog);

    // Остальной код создания файла
    jclass contentValuesClass = (*env)->FindClass(env, "android/content/ContentValues");
    if (contentValuesClass == NULL) {
        free(fullPath);
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        return strdup("error: ContentValues class not found");
    }

    jmethodID contentValuesInit = (*env)->GetMethodID(env, contentValuesClass, "<init>", "()V");
    if (contentValuesInit == NULL) {
        free(fullPath);
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        return strdup("error: ContentValues constructor not found");
    }

    jobject contentValues = (*env)->NewObject(env, contentValuesClass, contentValuesInit);
    if (contentValues == NULL) {
        free(fullPath);
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        return strdup("error: failed to create ContentValues");
    }

    jmethodID putString = (*env)->GetMethodID(env, contentValuesClass, "put", "(Ljava/lang/String;Ljava/lang/String;)V");

    if (putString == NULL) {
        free(fullPath);
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        return strdup("error: put methods not found");
    }

    // Устанавливаем имя файла
    jstring displayNameKey = (*env)->NewStringUTF(env, "_display_name");
    jstring displayNameValue = (*env)->NewStringUTF(env, baseName);
    (*env)->CallVoidMethod(env, contentValues, putString, displayNameKey, displayNameValue);
    (*env)->DeleteLocalRef(env, displayNameKey);
    (*env)->DeleteLocalRef(env, displayNameValue);

    // Устанавливаем MIME-тип
    jstring mimeTypeKey = (*env)->NewStringUTF(env, "mime_type");
    jstring mimeTypeValue = (*env)->NewStringUTF(env, mimeType);
    (*env)->CallVoidMethod(env, contentValues, putString, mimeTypeKey, mimeTypeValue);
    (*env)->DeleteLocalRef(env, mimeTypeKey);
    (*env)->DeleteLocalRef(env, mimeTypeValue);

    // Устанавливаем относительный путь если есть директории
    if (dirPath != NULL && strlen(dirPath) > 0) {
        jstring relativePathKey = (*env)->NewStringUTF(env, "relative_path");
        // Используем формат "Download/dirPath" для создания в подпапке Downloads
        char relativePath[512];
        snprintf(relativePath, sizeof(relativePath), "Download/%s", dirPath);
        jstring relativePathValue = (*env)->NewStringUTF(env, relativePath);
        (*env)->CallVoidMethod(env, contentValues, putString, relativePathKey, relativePathValue);
        (*env)->DeleteLocalRef(env, relativePathKey);
        (*env)->DeleteLocalRef(env, relativePathValue);

        char pathLog[256];
        snprintf(pathLog, sizeof(pathLog), "C: Setting relative path to: %s", relativePath);
        LogD(pathLog);
    }

    free(fullPath);

    // Создание файла через MediaStore
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

    if (uri == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        (*env)->DeleteLocalRef(env, mediaStoreClass);
        (*env)->DeleteLocalRef(env, collectionUri);
        (*env)->DeleteLocalRef(env, resolverClass);
        return strdup("error: MediaStore insert returned NULL");
    }

    // Получаем URI в виде строки
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

    // Очистка ресурсов
    (*env)->DeleteLocalRef(env, uriString);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, uriClass);
    (*env)->DeleteLocalRef(env, activityClass);
    (*env)->DeleteLocalRef(env, contentResolver);
    (*env)->DeleteLocalRef(env, contentValuesClass);
    (*env)->DeleteLocalRef(env, contentValues);
    (*env)->DeleteLocalRef(env, mediaStoreClass);
    (*env)->DeleteLocalRef(env, collectionUri);
    (*env)->DeleteLocalRef(env, resolverClass);

    return result;
}

// Для Android 7-9 (API 24-28)
static char* CreateFileInDownloadsLegacy(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType) {
    jclass environmentClass = NULL;
    jclass fileClass = NULL;
    jclass uriClass = NULL;
    jobject downloadsDirFile = NULL;
    jstring downloadsDirType = NULL;
    jstring fullPathStr = NULL;
    jobject fileObj = NULL;
    jobject uriObj = NULL;
    jstring uriString = NULL;

    const char* uriStr = NULL;
    char* result = NULL;

    environmentClass = (*env)->FindClass(env, "android/os/Environment");
    if (environmentClass == NULL) {
        result = strdup("error: Environment class not found");
        goto cleanup;
    }

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

    fileClass = (*env)->FindClass(env, "java/io/File");
    if (fileClass == NULL) {
        result = strdup("error: File class not found");
        goto cleanup;
    }

    jmethodID fileConstructor = (*env)->GetMethodID(env, fileClass, "<init>", "(Ljava/io/File;Ljava/lang/String;)V");
    if (fileConstructor == NULL) {
        result = strdup("error: File constructor not found");
        goto cleanup;
    }

    jstring fileNameStr = (*env)->NewStringUTF(env, fileName);
    fileObj = (*env)->NewObject(env, fileClass, fileConstructor, downloadsDirFile, fileNameStr);
    (*env)->DeleteLocalRef(env, fileNameStr);

    if (fileObj == NULL) {
        result = strdup("error: failed to create File object");
        goto cleanup;
    }

    jmethodID createNewFileMethod = (*env)->GetMethodID(env, fileClass, "createNewFile", "()Z");
    if (createNewFileMethod == NULL) {
        result = strdup("error: createNewFile method not found");
        goto cleanup;
    }

    jboolean success = (*env)->CallBooleanMethod(env, fileObj, createNewFileMethod);

    jmethodID toURIMethod = (*env)->GetMethodID(env, fileClass, "toURI", "()Ljava/net/URI;");
    if (toURIMethod == NULL) {
        result = strdup("error: toURI method not found");
        goto cleanup;
    }

    uriObj = (*env)->CallObjectMethod(env, fileObj, toURIMethod);
    if (uriObj == NULL) {
        result = strdup("error: uriObj == NULL");
        goto cleanup;
    }

    uriClass = (*env)->FindClass(env, "java/net/URI");
    if (uriClass == NULL) {
        result = strdup("error: URI class not found");
        goto cleanup;
    }

    jmethodID toStringMethod = (*env)->GetMethodID(env, uriClass, "toString", "()Ljava/lang/String;");
    if (toStringMethod == NULL) {
        result = strdup("error: toString method not found");
        goto cleanup;
    }

    uriString = (*env)->CallObjectMethod(env, uriObj, toStringMethod);
    if (uriString == NULL) {
        result = strdup("error: uriString == NULL");
        goto cleanup;
    }

    uriStr = (*env)->GetStringUTFChars(env, uriString, NULL);
    if (uriStr == NULL) {
        result = strdup("error: failed to get URI string");
        goto cleanup;
    }

    result = strdup(uriStr);

cleanup:
    if (uriStr != NULL) {
        (*env)->ReleaseStringUTFChars(env, uriString, uriStr);
    }

    if (environmentClass != NULL) (*env)->DeleteLocalRef(env, environmentClass);
    if (fileClass != NULL) (*env)->DeleteLocalRef(env, fileClass);
    if (uriClass != NULL) (*env)->DeleteLocalRef(env, uriClass);
    if (downloadsDirFile != NULL) (*env)->DeleteLocalRef(env, downloadsDirFile);
    if (downloadsDirType != NULL) (*env)->DeleteLocalRef(env, downloadsDirType);
    if (fullPathStr != NULL) (*env)->DeleteLocalRef(env, fullPathStr);
    if (fileObj != NULL) (*env)->DeleteLocalRef(env, fileObj);
    if (uriObj != NULL) (*env)->DeleteLocalRef(env, uriObj);
    if (uriString != NULL) (*env)->DeleteLocalRef(env, uriString);

    return result;
}

// Универсальная функция для всех версий Android
static char* CreateFileInDownloadsCompat(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType) {
    // Получаем версию Android
    jint api_level = get_api_level(env);

    char apiLog[64];
    snprintf(apiLog, sizeof(apiLog), "C: API level: %d", api_level);
    LogD(apiLog);

    if (api_level >= 29) {
        LogD("C: Using modern MediaStore approach");
        return CreateFileInDownloadsModern(env, activity, fileName, mimeType);
    } else {
        LogD("C: Using legacy file approach");
        if (!checkAndRequestStoragePermissions(env, activity)) {
            return strdup("error: Storage permission required");
        }
        return CreateFileInDownloadsLegacy(env, activity, fileName, mimeType);
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

// CreateFileInDownloads создает файл в папке Downloads с поддержкой всех версий Android
func CreateFileInDownloads(fileName, mimeType string) (string, error) {
	log.Debug("Creating file in Downloads: ", fileName)

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

		cResult := C.CreateFileInDownloadsCompat(env, activity, cFileName, cMimeType)
		if cResult == nil {
			err = errors.New("unknown error in JNI function")
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
		return "", errors.New("empty result from ContentResolver")
	}

	return result, nil
}

// ChildDownload создает файл и возвращает его для последующего наполнения данными
func ChildDownload(component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	// Создаем файл и получаем только URI
	uri, err := CreateFileInDownloads(component, "")
	if err != nil {
		err = fmt.Errorf("createFileInDownloads failed: %v", err)
		return
	}

	child, err = storage.ParseURI(uri)
	if err != nil {
		err = fmt.Errorf("parse URI failed: %v", err)
		return
	}

	return
}
