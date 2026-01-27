//go:build android

// set_mod_time_android.go
// func setModTime(uri fyne.URI, mtime time.Time) (bool, error) {return false, nil}
package main

/*
#include <jni.h>
#include <string.h>
#include <stdlib.h>
#include <android/log.h>
#include <sys/stat.h>
#include <errno.h>

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

// setModTimeUsingFD пытается установить время модификации файла, используя File Descriptor и futimens()
static jboolean setModTimeUsingFD(JNIEnv* env, jobject activity, const char* uriStr, jlong modTimeMillis) {
    jboolean success = JNI_FALSE;
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject pfd = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass resolverClass = NULL;
    jclass pfdClass = NULL;
    jstring juriStr = NULL;
    jstring modeStr = NULL;
    int fd = -1;

    // Инициализация указателей
    contentResolver = NULL;
    uri = NULL;
    pfd = NULL;
    activityClass = NULL;
    uriClass = NULL;
    resolverClass = NULL;
    pfdClass = NULL;
    juriStr = NULL;
    modeStr = NULL;

    // --- JNI Setup ---
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

    // --- Открытие File Descriptor ---
    jmethodID openFileDescriptorMethod = (*env)->GetMethodID(env, resolverClass, "openFileDescriptor", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/os/ParcelFileDescriptor;");
    if (openFileDescriptorMethod == NULL) goto cleanup;

    modeStr = (*env)->NewStringUTF(env, "rw");
    pfd = (*env)->CallObjectMethod(env, contentResolver, openFileDescriptorMethod, uri, modeStr);

    if (caseException(env, "openFileDescriptor") || pfd == NULL) {
        LogD("setModTimeUsingFD: Failed to get ParcelFileDescriptor");
        goto cleanup;
    }

    // --- Получение нативного файлового дескриптора ---
    pfdClass = (*env)->GetObjectClass(env, pfd);
    if (pfdClass == NULL) goto cleanup_pfd;

    jmethodID getFdMethod = (*env)->GetMethodID(env, pfdClass, "getFd", "()I");
    if (getFdMethod == NULL) goto cleanup_pfd;

    fd = (*env)->CallIntMethod(env, pfd, getFdMethod);
    if (caseException(env, "getFd") || fd < 0) {
        LogD("setModTimeUsingFD: Invalid file descriptor: %d", fd);
        goto cleanup_pfd;
    }

    // --- Использование futimens() ---
    long seconds = modTimeMillis / 1000;
    long nanoseconds = (modTimeMillis % 1000) * 1000000;

    struct timespec times[2];
    times[0].tv_sec = seconds;      // atime
    times[0].tv_nsec = nanoseconds;
    times[1].tv_sec = seconds;      // mtime
    times[1].tv_nsec = nanoseconds;

    if (futimens(fd, times) == 0) {
        LogD("setModTimeUsingFD: Successfully updated mod time via futimens()");
        success = JNI_TRUE;
    } else {
        LogD("setModTimeUsingFD: futimens() failed, errno: %d (%s)", errno, strerror(errno));
        // Permission denied - ожидаемая ошибка для многих провайдеров
    }

cleanup_pfd:
    // --- Очистка PFD ресурсов ---
    if (pfd) {
        jmethodID closeMethod = (*env)->GetMethodID(env, pfdClass, "close", "()V");
        if (closeMethod != NULL) {
            (*env)->CallVoidMethod(env, pfd, closeMethod);
            caseException(env, "close PFD");
        }
        (*env)->DeleteLocalRef(env, pfd);
        pfd = NULL; // Помечаем как очищенный
    }
    if (pfdClass) {
        (*env)->DeleteLocalRef(env, pfdClass);
        pfdClass = NULL;
    }

cleanup:
    // --- Безопасная очистка всех ресурсов ---
    if (modeStr) {
        (*env)->DeleteLocalRef(env, modeStr);
        modeStr = NULL;
    }
    if (uri) {
        (*env)->DeleteLocalRef(env, uri);
        uri = NULL;
    }
    if (juriStr) {
        (*env)->DeleteLocalRef(env, juriStr);
        juriStr = NULL;
    }
    if (contentResolver) {
        (*env)->DeleteLocalRef(env, contentResolver);
        contentResolver = NULL;
    }
    if (activityClass) {
        (*env)->DeleteLocalRef(env, activityClass);
        activityClass = NULL;
    }
    if (uriClass) {
        (*env)->DeleteLocalRef(env, uriClass);
        uriClass = NULL;
    }
    if (resolverClass) {
        (*env)->DeleteLocalRef(env, resolverClass);
        resolverClass = NULL;
    }

    return success;
}

// Основная функция setModTime
static jboolean setModTime(JNIEnv* env, jobject activity, const char* uriStr, jlong modTimeMillis) {
    jboolean success = JNI_FALSE;

    // --- Сначала пробуем стандартный подход через ContentResolver.update ---
    LogD("setModTime: Trying standard ContentResolver.update approach");

    jobject contentResolver = NULL;
    jobject uri = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass resolverClass = NULL;
    jclass contentValuesClass = NULL;
    jstring juriStr = NULL;
    jobject values = NULL;
    jclass longClass = NULL;
    jobject modTimeLong = NULL;
    jstring timeColumn = NULL;

    // Инициализация указателей
    contentResolver = NULL;
    uri = NULL;
    activityClass = NULL;
    uriClass = NULL;
    resolverClass = NULL;
    contentValuesClass = NULL;
    juriStr = NULL;
    values = NULL;
    longClass = NULL;
    modTimeLong = NULL;
    timeColumn = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) goto try_fallback;

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) goto try_fallback;

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) goto try_fallback;

    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) goto try_fallback;

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) goto try_fallback;

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) goto try_fallback;

    // Создаем ContentValues
    contentValuesClass = (*env)->FindClass(env, "android/content/ContentValues");
    if (contentValuesClass == NULL) goto try_fallback;

    jmethodID contentValuesConstructor = (*env)->GetMethodID(env, contentValuesClass, "<init>", "()V");
    if (contentValuesConstructor == NULL) goto try_fallback;

    values = (*env)->NewObject(env, contentValuesClass, contentValuesConstructor);
    if (caseException(env, "create ContentValues") || values == NULL) goto try_fallback;

    jmethodID putMethod = (*env)->GetMethodID(env, contentValuesClass, "put", "(Ljava/lang/String;Ljava/lang/Long;)V");
    if (putMethod == NULL) goto try_fallback;

    longClass = (*env)->FindClass(env, "java/lang/Long");
    if (longClass == NULL) goto try_fallback;

    jmethodID longConstructor = (*env)->GetMethodID(env, longClass, "<init>", "(J)V");
    if (longConstructor == NULL) goto try_fallback;

    modTimeLong = (*env)->NewObject(env, longClass, longConstructor, modTimeMillis);
    if (caseException(env, "create Long") || modTimeLong == NULL) goto try_fallback;

    // Пробуем разные колонки
    timeColumn = (*env)->NewStringUTF(env, "last_modified");
    (*env)->CallVoidMethod(env, values, putMethod, timeColumn, modTimeLong);

    if (caseException(env, "put last_modified")) {
        (*env)->ExceptionClear(env);
        LogD("setModTime: last_modified not writable, trying date_modified");

        (*env)->DeleteLocalRef(env, timeColumn);
        timeColumn = (*env)->NewStringUTF(env, "date_modified");
        (*env)->CallVoidMethod(env, values, putMethod, timeColumn, modTimeLong);

        if (caseException(env, "put date_modified")) {
            LogD("setModTime: date_modified also not writable, trying fallback");
            goto try_fallback;
        }
    }

    // Выполняем update
    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) goto try_fallback;

    jmethodID updateMethod = (*env)->GetMethodID(env, resolverClass, "update",
        "(Landroid/net/Uri;Landroid/content/ContentValues;Ljava/lang/String;[Ljava/lang/String;)I");
    if (updateMethod == NULL) goto try_fallback;

    jint rowsUpdated = (*env)->CallIntMethod(env, contentResolver, updateMethod, uri, values, NULL, NULL);

    if (caseException(env, "update mod time")) {
        LogD("setModTime: Update operation not supported, trying fallback");
        (*env)->ExceptionClear(env);
        goto try_fallback;
    } else if (rowsUpdated > 0) {
        success = JNI_TRUE;
        LogD("setModTime: Successfully updated mod time via ContentResolver");
        goto cleanup_standard;
    } else {
        LogD("setModTime: No rows updated, trying fallback");
        goto try_fallback;
    }

try_fallback:
    // --- Fallback: используем File Descriptor подход ---
    LogD("setModTime: Trying fallback approach with File Descriptor");

    // Безопасная очистка перед fallback
    if (timeColumn) {
        (*env)->DeleteLocalRef(env, timeColumn);
        timeColumn = NULL;
    }
    if (modTimeLong) {
        (*env)->DeleteLocalRef(env, modTimeLong);
        modTimeLong = NULL;
    }
    if (values) {
        (*env)->DeleteLocalRef(env, values);
        values = NULL;
    }
    if (contentValuesClass) {
        (*env)->DeleteLocalRef(env, contentValuesClass);
        contentValuesClass = NULL;
    }
    if (longClass) {
        (*env)->DeleteLocalRef(env, longClass);
        longClass = NULL;
    }

    success = setModTimeUsingFD(env, activity, uriStr, modTimeMillis);

cleanup_standard:
    // Безопасная очистка оставшихся ресурсов
    if (timeColumn) {
        (*env)->DeleteLocalRef(env, timeColumn);
        timeColumn = NULL;
    }
    if (modTimeLong) {
        (*env)->DeleteLocalRef(env, modTimeLong);
        modTimeLong = NULL;
    }
    if (values) {
        (*env)->DeleteLocalRef(env, values);
        values = NULL;
    }
    if (uri) {
        (*env)->DeleteLocalRef(env, uri);
        uri = NULL;
    }
    if (juriStr) {
        (*env)->DeleteLocalRef(env, juriStr);
        juriStr = NULL;
    }
    if (contentResolver) {
        (*env)->DeleteLocalRef(env, contentResolver);
        contentResolver = NULL;
    }
    if (activityClass) {
        (*env)->DeleteLocalRef(env, activityClass);
        activityClass = NULL;
    }
    if (uriClass) {
        (*env)->DeleteLocalRef(env, uriClass);
        uriClass = NULL;
    }
    if (resolverClass) {
        (*env)->DeleteLocalRef(env, resolverClass);
        resolverClass = NULL;
    }
    if (contentValuesClass) {
        (*env)->DeleteLocalRef(env, contentValuesClass);
        contentValuesClass = NULL;
    }
    if (longClass) {
        (*env)->DeleteLocalRef(env, longClass);
        longClass = NULL;
    }

    return success;
}
*/
import "C"
import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

// setModTime устанавливает время модификации файла через ContentResolver
// Возвращает true если успешно, false если операция не поддерживается
func setModTime(uri fyne.URI, mtime time.Time) (bool, error) {
	if uri == nil {
		return false, fmt.Errorf("uri is nil")
	}

	var success bool

	err := driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		// Преобразуем time.Time в миллисекунды
		modTimeMillis := mtime.UnixMilli()

		cSuccess := C.setModTime(env, activity, uriStr, C.jlong(modTimeMillis))
		success = (cSuccess != 0)

		return nil
	})

	if err != nil {
		return false, fmt.Errorf("native execution failed: %v", err)
	}

	return success, nil
}

func SetModTime(uri fyne.URI, mtime time.Time) error {
	if uri == nil {
		return fmt.Errorf("uri is nil")
	}

	// Для обычных файлов используем стандартный подход
	if uri.Scheme() != "content" {
		return os.Chtimes(uri.Path(), time.Time{}, mtime)
	}

	// Для content URI пытаемся установить время модификации
	success, err := setModTime(uri, mtime)
	if err != nil {
		return fmt.Errorf("failed to set modification time: %v", err)
	}

	if !success {
		// Это не ошибка - многие провайдеры просто не поддерживают эту операцию
		log.Debugf("Setting modification time not supported for URI: %s", uri)
	} else {
		log.Debugf("Successfully set modification time for URI: %s", uri)
	}

	return nil
}
