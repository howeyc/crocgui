//go:build android

// get_mod_time_android.go
// func getModTime(uri fyne.URI) (modTime int64, err error) {return 0, nil}
package main

/*
#include <jni.h>
#include <string.h>
#include <android/log.h>
#include <stdlib.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)

// Вспомогательная функция для проверки и очистки исключений
static jboolean caseException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogD("Exception in %s", context);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE;
    }
    return JNI_FALSE;
}

static jlong getModTime(JNIEnv* env, jobject activity, const char* uriStr) {
    jlong modTime = -1;
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject cursor = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass resolverClass = NULL;
    jclass cursorClass = NULL;
    jclass stringClass = NULL;
    jstring juriStr = NULL;
    jstring colNameStr = NULL; // Используем одно имя для обеих попыток
    jobjectArray projection = NULL;
    jmethodID closeMethod = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) goto cleanup;
    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) goto cleanup;
    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) goto cleanup;
    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) goto cleanup;
    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) goto cleanup;
    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) goto cleanup;
    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) goto cleanup;
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) goto cleanup;
    stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) goto cleanup;

    // --- Попытка 1: Используем "last_modified" (Ваш рабочий вариант) ---
    colNameStr = (*env)->NewStringUTF(env, "last_modified");
    projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    (*env)->SetObjectArrayElement(env, projection, 0, colNameStr);

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, uri, projection, NULL, NULL, NULL);

    // Если запрос завершился неудачно (исключение ИЛИ NULL-курсор)
    if (caseException(env, "query for last_modified") || cursor == NULL) {
        LogD("getModTime: LAST_MODIFIED query failed, trying date_modified");

        // Очищаем ресурсы первой попытки
        if (cursor) (*env)->DeleteLocalRef(env, cursor);
        cursor = NULL; // Сбрасываем курсор
        (*env)->DeleteLocalRef(env, colNameStr); // Удаляем старое имя

        // --- Попытка 2: Используем "date_modified" ---
        colNameStr = (*env)->NewStringUTF(env, "date_modified");
        (*env)->SetObjectArrayElement(env, projection, 0, colNameStr); // Переиспользуем projection array

        cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, uri, projection, NULL, NULL, NULL);

        if (caseException(env, "query for date_modified") || cursor == NULL) {
            LogD("getModTime: DATE_MODIFIED query also failed");
            goto cleanup;
        }
    }
    // К этому моменту либо cursor из первой попытки, либо из второй попытки содержит валидный курсор.

    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) goto cleanup;

    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getLong = (*env)->GetMethodID(env, cursorClass, "getLong", "(I)J");
    closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");

    if (moveToFirst == NULL || getLong == NULL || closeMethod == NULL) goto cleanup;

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        modTime = (*env)->CallLongMethod(env, cursor, getLong, 0); // Получаем long value

        if (caseException(env, "getLong for mod time")) {
            modTime = -1;
            LogD("getModTime: Failed to get long value for mod time");
        } else {
            // modTime теперь содержит миллисекунды (как подтвердили ваши логи)
            LogD("getModTime: Got mod time: %lld", (long long)modTime);
        }
    } else {
        LogD("getModTime: No mod time available");
    }

cleanup:
    if (cursor) {
        if (closeMethod != NULL) {
            (*env)->CallVoidMethod(env, cursor, closeMethod);
            caseException(env, "close cursor in getModTime");
        }
        (*env)->DeleteLocalRef(env, cursor);
    }
    if (projection) (*env)->DeleteLocalRef(env, projection);
    if (colNameStr) (*env)->DeleteLocalRef(env, colNameStr);
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (juriStr) (*env)->DeleteLocalRef(env, juriStr);
    if (contentResolver) (*env)->DeleteLocalRef(env, contentResolver);
    if (activityClass) (*env)->DeleteLocalRef(env, activityClass);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (resolverClass) (*env)->DeleteLocalRef(env, resolverClass);
    if (cursorClass) (*env)->DeleteLocalRef(env, cursorClass);
    if (stringClass) (*env)->DeleteLocalRef(env, stringClass);

    return modTime;
}
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

// getModTime возвращает время модификации файла в миллисекундах с эпохи Unix
func getModTime(uri fyne.URI) (modTime int64, err error) {
	if uri == nil {
		return 0, fmt.Errorf("uri is nil")
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cModTime := C.getModTime(env, activity, uriStr)
		modTime = int64(cModTime)

		if modTime == -1 {
			modTime = 0
			err = fmt.Errorf("failed to get modification time")
		}

		return nil
	})

	return modTime, err
}

// ModTime возвращает время модификации файла как time.Time
func ModTime(uri fyne.URI) (time.Time, error) {
	if uri == nil {
		return time.Time{}, fmt.Errorf("uri is nil")
	}

	// Для обычных файлов используем стандартный подход
	if uri.Scheme() != "content" {
		return fileModTime(uri.Path())
	}

	// Для content URI используем Android-специфичный метод
	modTimeMs, err := getModTime(uri)
	if err != nil {
		return time.Time{}, err
	}

	if modTimeMs == -1 || modTimeMs == 0 {
		return time.Time{}, fmt.Errorf("modification time not available")
	}

	// Создаем time.Time из миллисекунд (ваш C-код уже сделал преобразование)
	return time.UnixMilli(modTimeMs), nil
}
