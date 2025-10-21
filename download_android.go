//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

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

// Объявляем функции ДО их использования
jint get_api_level(JNIEnv* env);

static char* CreateFileInDownloadsModern(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType);
static char* CreateFileInDownloadsLegacy(JNIEnv* env, jobject activity, const char* fileName, const char* mimeType);

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

// Для Android 10+ (API 29+)
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

    jmethodID putString = (*env)->GetMethodID(env, contentValuesClass, "put", "(Ljava/lang/String;Ljava/lang/String;)V");
    if (putString == NULL) {
        free(fullPath);
        (*env)->DeleteLocalRef(env, activityClass);
        (*env)->DeleteLocalRef(env, contentResolver);
        (*env)->DeleteLocalRef(env, contentValuesClass);
        (*env)->DeleteLocalRef(env, contentValues);
        return strdup("error: put method not found");
    }

    jstring displayNameKey = (*env)->NewStringUTF(env, "_display_name");
    jstring displayNameValue = (*env)->NewStringUTF(env, baseName);
    (*env)->CallVoidMethod(env, contentValues, putString, displayNameKey, displayNameValue);
    (*env)->DeleteLocalRef(env, displayNameKey);
    (*env)->DeleteLocalRef(env, displayNameValue);

    jstring mimeTypeKey = (*env)->NewStringUTF(env, "mime_type");
    jstring mimeTypeValue = (*env)->NewStringUTF(env, mimeType);
    (*env)->CallVoidMethod(env, contentValues, putString, mimeTypeKey, mimeTypeValue);
    (*env)->DeleteLocalRef(env, mimeTypeKey);
    (*env)->DeleteLocalRef(env, mimeTypeValue);

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
	log.Trace("Creating file in Downloads: " + fileName)

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
			err = errors.New(strings.TrimPrefix(resultStr, "error: "))
		} else {
			result = resultStr
		}
		return nil
	})

	if err != nil {
		log.Error("Failed to create file: " + err.Error())
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	if result == "" {
		return "", errors.New("empty result from ContentResolver")
	}

	return result, nil
}

func ChildDownload(component string) (child fyne.URI, cleanup func(), err error) {
	cleanup = func() {}

	newFileURL, err := CreateFileInDownloads(component, "")
	if err != nil {
		err = fmt.Errorf("createFileInDownloads failed: %v", err)
		return
	}

	child, err = storage.ParseURI(newFileURL)
	if err != nil {
		err = fmt.Errorf("parse URI failed: %v", err)
		return
	}

	return
}
