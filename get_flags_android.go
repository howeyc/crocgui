//go:build android

// getflags_android.go
// func getFlags(){}
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

// getFlags пытается получить флаги документа
static jint getFlags(JNIEnv* env, jobject activity, const char* uriStr) {
    jint flags = -1;
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject cursor = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass resolverClass = NULL;
    jclass cursorClass = NULL;
    jstring juriStr = NULL;
    jstring flagsCol = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("getFlags: Failed to get activity class");
        goto cleanup;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("getFlags: Failed to get getContentResolver method");
        goto cleanup;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) {
        LogD("getFlags: Failed to get contentResolver");
        goto cleanup;
    }

    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("getFlags: Failed to find Uri class");
        goto cleanup;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("getFlags: Failed to get parse method");
        goto cleanup;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) {
        LogD("getFlags: Failed to parse URI");
        goto cleanup;
    }

    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        LogD("getFlags: Failed to get resolver class");
        goto cleanup;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("getFlags: Failed to get query method");
        goto cleanup;
    }

    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("getFlags: Failed to find String class");
        goto cleanup;
    }

    flagsCol = (*env)->NewStringUTF(env, "flags");
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    (*env)->SetObjectArrayElement(env, projection, 0, flagsCol);

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, uri, projection, NULL, NULL, NULL);
    if (caseException(env, "query for flags") || cursor == NULL) {
        LogD("getFlags: FLAGS query returned NULL cursor");
        goto cleanup;
    }

    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        LogD("getFlags: Failed to get cursor class");
        goto cleanup;
    }

    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getInt = (*env)->GetMethodID(env, cursorClass, "getInt", "(I)I");

    if (moveToFirst == NULL || getInt == NULL) {
        LogD("getFlags: Failed to get cursor methods");
        goto cleanup;
    }

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        flags = (*env)->CallIntMethod(env, cursor, getInt, 0);
        if (caseException(env, "getInt for flags")) {
            flags = -1;
        } else {
            LogD("getFlags: Got flags: %d", flags);
        }
    }

cleanup:
    if (cursor) {
        if (cursorClass != NULL) {
            jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
            if (closeMethod != NULL) {
                (*env)->CallVoidMethod(env, cursor, closeMethod);
                caseException(env, "close cursor in getFlags");
            }
        }
        (*env)->DeleteLocalRef(env, cursor);
    }
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (juriStr) (*env)->DeleteLocalRef(env, juriStr);
    if (flagsCol) (*env)->DeleteLocalRef(env, flagsCol);
    if (activityClass) (*env)->DeleteLocalRef(env, activityClass);
    if (contentResolver) (*env)->DeleteLocalRef(env, contentResolver);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (resolverClass) (*env)->DeleteLocalRef(env, resolverClass);
    if (cursorClass) (*env)->DeleteLocalRef(env, cursorClass);

    return flags;
}
*/
import "C"
import (
	"fmt"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

// getFlags возвращает флаги документа
func getFlags(uri fyne.URI) (flags int, err error) {
	if uri == nil {
		return 0, fmt.Errorf("uri is nil")
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cFlags := C.getFlags(env, activity, uriStr)
		flags = int(cFlags)

		if flags == -1 {
			flags = 0
			err = fmt.Errorf("failed to get flags")
		}

		return nil
	})

	return flags, err
}

// printFlags печатает подробную информацию о флагах документа
func printFlags(uri fyne.URI) {
	if uri == nil {
		log.Debugf("printFlags: URI is nil")
		return
	}

	flags, err := getFlags(uri)
	if err != nil {
		log.Debugf("printFlags: Failed to get flags for %s: %v", uri, err)
		return
	}

	if flags == 0 {
		log.Debugf("printFlags: No flags available for %s", uri)
		return
	}

	log.Debugf("printFlags: Raw flags value: %d (0x%08X)", flags, flags)
	log.Debugf("printFlags: Detailed flags for %s:", uri)

	// Основные флаги
	flagDefinitions := []struct {
		flag        int
		name        string
		description string
	}{
		{0x00000001, "FLAG_SUPPORTS_DELETE", "Поддержка удаления"},
		{0x00000002, "FLAG_SUPPORTS_WRITE", "Поддержка записи"},
		{0x00000004, "FLAG_SUPPORTS_RENAME", "Поддержка переименования"},
		{0x00000008, "FLAG_SUPPORTS_MOVED", "Поддержка перемещения"},
		{0x00000010, "FLAG_DIR_PREFERS_GRID", "Предпочтение сетки для директорий"},
		{0x00000040, "FLAG_SUPPORTS_COPY", "Поддержка копирования"},
		{0x00000080, "FLAG_SUPPORTS_MOVE", "Поддержка перемещения"},
		{0x00000100, "FLAG_DIR_SUPPORTS_CREATE", "Поддержка создания в директориях"},
		{0x00000200, "FLAG_SUPPORTS_REMOVE", "Поддержка удаления"},
		{0x00000400, "FLAG_SUPPORTS_ADD", "Поддержка добавления"},
		{0x00000800, "FLAG_SUPPORTS_BLOCK_REMOVE", "Поддержка блочного удаления"},
		{0x00001000, "FLAG_SUPPORTS_BLOCK_ADD", "Поддержка блочного добавления"},
		{0x00002000, "FLAG_SUPPORTS_SEEK", "Поддержка поиска (seek)"},
		{0x00004000, "FLAG_SUPPORTS_BLOCK_TRANSFER", "Поддержка блочной передачи"},
		{0x00008000, "FLAG_PARTIAL_UPDATES", "Частичные обновления"},
		{0x00010000, "FLAG_VIRTUAL_DOCUMENT", "Виртуальный документ"},
		{0x00020000, "FLAG_PARTIAL_DOCUMENT", "Частичный документ"},
		{0x00040000, "FLAG_SUPPORTS_SETTINGS", "Поддержка настроек"},
		{0x00080000, "FLAG_SUPPORTS_CLEAR_METADATA", "Поддержка очистки метаданных"},
		{0x00100000, "FLAG_SUPPORTS_RESTORE", "Поддержка восстановления"},
		{0x00200000, "FLAG_SUPPORTS_PIN", "Поддержка закрепления"},
		{0x00400000, "FLAG_SUPPORTS_UNPIN", "Поддержка открепления"},
		{0x00800000, "FLAG_PINNED", "Закреплен"},
		{0x01000000, "FLAG_SUPPORTS_SHOW_IN_APP", "Поддержка показа в приложении"},
		{0x02000000, "FLAG_SUPPORTS_EJECT", "Поддержка извлечения"},
		{0x04000000, "FLAG_SUPPORTS_FORMAT", "Поддержка форматирования"},
		{0x08000000, "FLAG_CACHED", "Кэширован"},
		{0x10000000, "FLAG_SUPPORTS_RECENTS", "Поддержка недавних документов"},
		{0x20000000, "FLAG_SUPPORTS_INFO", "Поддержка информации"},
		{0x40000000, "FLAG_SUPPORTS_PLAY", "Поддержка воспроизведения"},
		// {0x80000000, "FLAG_PLAYING", "Воспроизводится"},
	}

	// Собираем установленные флаги
	var setFlags []string
	var setFlagValues []int

	for _, def := range flagDefinitions {
		if flags&def.flag != 0 {
			setFlags = append(setFlags, fmt.Sprintf("%s (0x%08X) - %s", def.name, def.flag, def.description))
			setFlagValues = append(setFlagValues, def.flag)
		}
	}

	if len(setFlags) > 0 {
		log.Debugf("printFlags: Set flags (%d):", len(setFlags))
		for i, flag := range setFlags {
			log.Debugf("printFlags:   [%2d] %s", i+1, flag)
		}
	} else {
		log.Debugf("printFlags: No flags set")
	}

	// Проверяем сумму установленных флагов
	if len(setFlagValues) > 0 {
		sum := 0
		for _, val := range setFlagValues {
			sum |= val
		}
		if sum == flags {
			log.Debugf("printFlags: Flag sum verification: OK (0x%08X)", sum)
		} else {
			log.Debugf("printFlags: Flag sum verification: MISMATCH! Calculated: 0x%08X, Actual: 0x%08X", sum, flags)
			log.Debugf("printFlags: There might be unknown flags set: 0x%08X", flags^sum)
		}
	}

	// Информация о правах доступа
	writeAccess := flags&0x00000002 != 0
	deleteAccess := flags&0x00000001 != 0
	readAccess := flags&0x00000002 != 0 // Обычно WRITE подразумевает и READ

	log.Debugf("printFlags: Access - Read: %v, Write: %v, Delete: %v",
		readAccess, writeAccess, deleteAccess)
}
