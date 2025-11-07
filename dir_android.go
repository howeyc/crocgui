//go:build android

// dir_android.go
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
        return JNI_TRUE; // Было исключение
    }
    return JNI_FALSE; // Не было исключения
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

// getSize пытается получить размер файла
static jlong getSize(JNIEnv* env, jobject activity, const char* uriStr) {
    jlong size = -1;
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject cursor = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass openableColumnsClass = NULL;
    jclass resolverClass = NULL;
    jclass cursorClass = NULL;
    jstring juriStr = NULL;
    jstring sizeCol = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("getSize: Failed to get activity class");
        goto cleanup;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("getSize: Failed to get getContentResolver method");
        goto cleanup;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) {
        LogD("getSize: Failed to get contentResolver");
        goto cleanup;
    }

    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("getSize: Failed to find Uri class");
        goto cleanup;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("getSize: Failed to get parse method");
        goto cleanup;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) {
        LogD("getSize: Failed to parse URI");
        goto cleanup;
    }

    openableColumnsClass = (*env)->FindClass(env, "android/provider/OpenableColumns");
    if (openableColumnsClass == NULL) {
        LogD("getSize: Failed to find OpenableColumns class");
        goto cleanup;
    }

    jfieldID sizeFieldID = (*env)->GetStaticFieldID(env, openableColumnsClass, "SIZE", "Ljava/lang/String;");
    if (sizeFieldID == NULL) {
        LogD("getSize: Failed to get SIZE field ID");
        goto cleanup;
    }

    sizeCol = (*env)->GetStaticObjectField(env, openableColumnsClass, sizeFieldID);

    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        LogD("getSize: Failed to get resolver class");
        goto cleanup;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("getSize: Failed to get query method");
        goto cleanup;
    }

    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("getSize: Failed to find String class");
        goto cleanup;
    }

    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    (*env)->SetObjectArrayElement(env, projection, 0, sizeCol);

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, uri, projection, NULL, NULL, NULL);
    if (caseException(env, "query for size") || cursor == NULL) {
        LogD("getSize: SIZE query returned NULL cursor");
        goto cleanup;
    }

    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        LogD("getSize: Failed to get cursor class");
        goto cleanup;
    }

    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getLong = (*env)->GetMethodID(env, cursorClass, "getLong", "(I)J");

    if (moveToFirst == NULL || getLong == NULL) {
        LogD("getSize: Failed to get cursor methods");
        goto cleanup;
    }

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        size = (*env)->CallLongMethod(env, cursor, getLong, 0);
        if (caseException(env, "getLong for size")) {
            size = -1;
        } else {
            LogD("getSize: Got size: %lld", (long long)size);
        }
    } else {
        LogD("getSize: No size available");
    }

cleanup:
    if (cursor) {
        if (cursorClass != NULL) {
            jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
            if (closeMethod != NULL) {
                (*env)->CallVoidMethod(env, cursor, closeMethod);
                caseException(env, "close cursor in getSize");
            }
        }
        (*env)->DeleteLocalRef(env, cursor);
    }
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (juriStr) (*env)->DeleteLocalRef(env, juriStr);
    if (sizeCol) (*env)->DeleteLocalRef(env, sizeCol);
    if (activityClass) (*env)->DeleteLocalRef(env, activityClass);
    if (contentResolver) (*env)->DeleteLocalRef(env, contentResolver);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (openableColumnsClass) (*env)->DeleteLocalRef(env, openableColumnsClass);
    if (resolverClass) (*env)->DeleteLocalRef(env, resolverClass);
    if (cursorClass) (*env)->DeleteLocalRef(env, cursorClass);

    return size;
}

// getMime пытается получить MIME-тип
static char* getMime(JNIEnv* env, jobject activity, const char* uriStr) {
    char* mimeTypeStr = NULL;
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject cursor = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass resolverClass = NULL;
    jclass cursorClass = NULL;
    jstring juriStr = NULL;
    jstring mimeTypeCol = NULL;
    jstring jMime = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("getMime: Failed to get activity class");
        goto cleanup;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("getMime: Failed to get getContentResolver method");
        goto cleanup;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) {
        LogD("getMime: Failed to get contentResolver");
        goto cleanup;
    }

    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("getMime: Failed to find Uri class");
        goto cleanup;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("getMime: Failed to get parse method");
        goto cleanup;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) {
        LogD("getMime: Failed to parse URI");
        goto cleanup;
    }

    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        LogD("getMime: Failed to get resolver class");
        goto cleanup;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("getMime: Failed to get query method");
        goto cleanup;
    }

    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("getMime: Failed to find String class");
        goto cleanup;
    }

    mimeTypeCol = (*env)->NewStringUTF(env, "mime_type");
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    (*env)->SetObjectArrayElement(env, projection, 0, mimeTypeCol);

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, uri, projection, NULL, NULL, NULL);
    if (caseException(env, "query for MIME type") || cursor == NULL) {
        LogD("getMime: MIME query returned NULL cursor");
        goto cleanup;
    }

    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        LogD("getMime: Failed to get cursor class");
        goto cleanup;
    }

    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");

    if (moveToFirst == NULL || getString == NULL) {
        LogD("getMime: Failed to get cursor methods");
        goto cleanup;
    }

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        jMime = (jstring)(*env)->CallObjectMethod(env, cursor, getString, 0);
        if (caseException(env, "getString for MIME type")) {
            // Исключение при получении строки
            jMime = NULL;
        }

        if (jMime != NULL) {
            const char* mimeStr = (*env)->GetStringUTFChars(env, jMime, NULL);
            if (mimeStr != NULL) {
                mimeTypeStr = strdup(mimeStr);
                LogD("getMime: Got MIME type: %s", mimeStr);
                (*env)->ReleaseStringUTFChars(env, jMime, mimeStr);
            }
        }
    }

cleanup:
    if (cursor) {
        if (cursorClass != NULL) {
            jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
            if (closeMethod != NULL) {
                (*env)->CallVoidMethod(env, cursor, closeMethod);
                caseException(env, "close cursor in getMime");
            }
        }
        (*env)->DeleteLocalRef(env, cursor);
    }
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (juriStr) (*env)->DeleteLocalRef(env, juriStr);
    if (jMime) (*env)->DeleteLocalRef(env, jMime);
    if (mimeTypeCol) (*env)->DeleteLocalRef(env, mimeTypeCol);
    if (activityClass) (*env)->DeleteLocalRef(env, activityClass);
    if (contentResolver) (*env)->DeleteLocalRef(env, contentResolver);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (resolverClass) (*env)->DeleteLocalRef(env, resolverClass);
    if (cursorClass) (*env)->DeleteLocalRef(env, cursorClass);

    return mimeTypeStr;
}

static jint countChildren(JNIEnv* env, jobject activity, const char* uriStr) {
    jint count = -1;
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject cursor = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass documentsContractClass = NULL;
    jclass resolverClass = NULL;
    jclass cursorClass = NULL;
    jstring juriStr = NULL;
    jobject childUri = NULL;
    jboolean childUriNeedsCleanup = JNI_FALSE;

    // Инициализация ContentResolver
    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("countChildren: Failed to get activity class");
        goto cleanup;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("countChildren: Failed to get getContentResolver method");
        goto cleanup;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver")) {
        goto cleanup;
    }
    if (contentResolver == NULL) {
        LogD("countChildren: contentResolver is NULL");
        goto cleanup;
    }

    // Парсинг URI
    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("countChildren: Failed to find Uri class");
        goto cleanup;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("countChildren: Failed to get parse method");
        goto cleanup;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI")) {
        goto cleanup;
    }
    if (uri == NULL) {
        LogD("countChildren: parse returned NULL");
        goto cleanup;
    }

    // Поиск DocumentsContract класса
    documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogD("countChildren: Failed to find DocumentsContract class");
        goto cleanup;
    }

    // Кэширование методов DocumentsContract
    jmethodID buildChildDocumentsUriUsingTreeMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
    jmethodID getTreeDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "getTreeDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");
    jmethodID buildChildDocumentsUriMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildChildDocumentsUri", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
    jmethodID getDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "getDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");

    // Метод 1: buildChildDocumentsUriUsingTree (основной)
    if (buildChildDocumentsUriUsingTreeMethod != NULL && getTreeDocumentIdMethod != NULL) {
        jstring treeDocId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getTreeDocumentIdMethod, uri);
        if (caseException(env, "getTreeDocumentId")) {
            LogD("countChildren: getTreeDocumentId failed with exception");
        } else if (treeDocId != NULL) {
            childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                buildChildDocumentsUriUsingTreeMethod, uri, treeDocId);
            if (caseException(env, "buildChildDocumentsUriUsingTree")) {
                LogD("countChildren: buildChildDocumentsUriUsingTree failed with exception");
                if (childUri != NULL) {
                    (*env)->DeleteLocalRef(env, childUri);
                    childUri = NULL;
                }
            } else if (childUri != NULL) {
                childUriNeedsCleanup = JNI_TRUE;
                LogD("countChildren: Successfully built child URI using tree method");
            }
            (*env)->DeleteLocalRef(env, treeDocId);
        }
    }

    // Метод 2: buildChildDocumentsUri (альтернативный)
    if (childUri == NULL && buildChildDocumentsUriMethod != NULL && getDocumentIdMethod != NULL) {
        jstring docId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getDocumentIdMethod, uri);
        if (caseException(env, "getDocumentId")) {
            LogD("countChildren: getDocumentId failed with exception");
        } else if (docId != NULL) {
            childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                buildChildDocumentsUriMethod, uri, docId);
            if (caseException(env, "buildChildDocumentsUri")) {
                LogD("countChildren: buildChildDocumentsUri failed with exception");
                if (childUri != NULL) {
                    (*env)->DeleteLocalRef(env, childUri);
                    childUri = NULL;
                }
            } else if (childUri != NULL) {
                childUriNeedsCleanup = JNI_TRUE;
                LogD("countChildren: Successfully built child URI using document method");
            }
            (*env)->DeleteLocalRef(env, docId);
        }
    }

    // Метод 3: Прямой query исходного URI (fallback)
    if (childUri == NULL) {
        LogD("countChildren: Using direct URI query as fallback");
        childUri = (*env)->NewLocalRef(env, uri);
        if (childUri != NULL) {
            childUriNeedsCleanup = JNI_TRUE;
        }
    }

    if (childUri == NULL) {
        LogD("countChildren: Failed to build child URI");
        count = -7;
        goto cleanup;
    }

    // Выполнение запроса
    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        LogD("countChildren: Failed to get resolver class");
        count = -6;
        goto cleanup;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass,
        "query", "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("countChildren: Failed to get query method");
        count = -6;
        goto cleanup;
    }

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, NULL, NULL, NULL, NULL);
    if (caseException(env, "query for children")) {
        LogD("countChildren: Query failed with exception");
        count = -8;
        goto cleanup;
    }
    if (cursor == NULL) {
        LogD("countChildren: Query returned NULL cursor");
        count = -5;
        goto cleanup;
    }

    // Получение количества записей
    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        LogD("countChildren: Failed to get cursor class");
        count = -4;
        goto cleanup;
    }

    jmethodID getCount = (*env)->GetMethodID(env, cursorClass, "getCount", "()I");
    if (getCount == NULL) {
        LogD("countChildren: Failed to get getCount method");
        count = -3;
        goto cleanup;
    }

    count = (*env)->CallIntMethod(env, cursor, getCount);
    if (caseException(env, "getCount")) {
        LogD("countChildren: getCount failed with exception");
        count = -9;
        goto cleanup;
    }

    LogD("countChildren: Successfully got count: %d", count);

cleanup:
    // Закрытие курсора (если не был закрыт ранее)
    if (cursor != NULL) {
        if (cursorClass != NULL) {
            jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
            if (closeMethod != NULL) {
                (*env)->CallVoidMethod(env, cursor, closeMethod);
                caseException(env, "close cursor in cleanup");
            }
        }
        (*env)->DeleteLocalRef(env, cursor);
    }

    // Освобождение остальных ресурсов
    if (childUri != NULL && childUriNeedsCleanup) {
        (*env)->DeleteLocalRef(env, childUri);
    }
    if (uri != NULL) (*env)->DeleteLocalRef(env, uri);
    if (juriStr != NULL) (*env)->DeleteLocalRef(env, juriStr);
    if (activityClass != NULL) (*env)->DeleteLocalRef(env, activityClass);
    if (contentResolver != NULL) (*env)->DeleteLocalRef(env, contentResolver);
    if (uriClass != NULL) (*env)->DeleteLocalRef(env, uriClass);
    if (documentsContractClass != NULL) (*env)->DeleteLocalRef(env, documentsContractClass);
    if (resolverClass != NULL) (*env)->DeleteLocalRef(env, resolverClass);
    if (cursorClass != NULL) (*env)->DeleteLocalRef(env, cursorClass);

    return count;
}
*/
import "C"
import (
	"fmt"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/storage"
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

// getSize возвращает размер файла в байтах
func getSize(uri fyne.URI) (size int64, err error) {
	if uri == nil {
		return 0, fmt.Errorf("uri is nil")
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cSize := C.getSize(env, activity, uriStr)
		size = int64(cSize)

		if size == -1 {
			size = 0
			err = fmt.Errorf("failed to get size")
		}

		return nil
	})

	return size, err
}

// mimeType возвращает MIME-тип URI
func mimeType(uri fyne.URI) (mimeTypeStr string) {
	if uri == nil {
		return
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cMimeStr := C.getMime(env, activity, uriStr)
		if cMimeStr != nil {
			mimeTypeStr = C.GoString(cMimeStr)
			C.free(unsafe.Pointer(cMimeStr))
		}

		return nil
	})

	return mimeTypeStr
}
func countChild(uri fyne.URI) (count int, err error) {
	if uri == nil {
		return 0, fmt.Errorf("uri is nil")
	}

	// Проверяем, можно ли получить список содержимого для данного URI
	listable, err := storage.CanList(uri)
	if err != nil {
		return 0, fmt.Errorf("cannot check if URI is listable: %w", err)
	}
	if !listable {
		return 0, fmt.Errorf("URI is not a listable directory: %s", uri.String())
	}

	// Получаем список дочерних элементов
	children, err := storage.List(uri)
	if err != nil {
		return 0, fmt.Errorf("failed to list directory contents: %w", err)
	}

	return len(children), nil
}

// countChild возвращает количество дочерних элементов в директории DocumentsContract
func countChild0(uri fyne.URI) (count int, err error) {
	if uri == nil {
		return 0, fmt.Errorf("uri is nil")
	}

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		cCount := C.countChildren(env, activity, uriStr)
		count = int(cCount)

		// Обработка кодов ошибок с детализацией
		if count < 0 {
			switch count {
			case -1:
				err = fmt.Errorf("general failure: failed to initialize or parse URI")
			case -3:
				err = fmt.Errorf("cursor operation failed: getCount method not available")
			case -4:
				err = fmt.Errorf("cursor operation failed: cursor class not available")
			case -5:
				err = fmt.Errorf("query failed: returned NULL cursor - no permissions or invalid URI")
			case -6:
				err = fmt.Errorf("query method not available: ContentResolver query method not found")
			case -7:
				err = fmt.Errorf("URI construction failed: cannot build child documents URI")
			case -8:
				err = fmt.Errorf("query execution failed: exception during database query")
			case -9:
				err = fmt.Errorf("count retrieval failed: exception during getCount operation")
			default:
				err = fmt.Errorf("unknown error code: %d", count)
			}
			count = 0
		} else {
			// Успешное выполнение
			log.Tracef("countChildren: successfully counted %d children for URI: %s", count, uri.String())
		}

		return nil
	})

	return count, err
}
func IsDirectory(uri fyne.URI) bool {
	if uri == nil {
		return false
	}
	switch mimeType(uri) {
	case "vnd.android.document/directory":
		return true
	case "", "application/octet-stream":
		size, sizeErr := getSize(uri)
		if sizeErr == nil && size == 4096 &&
			strings.HasPrefix(uri.String(), ZhangHai) {
			return true
		}
		_, err := countChild(uri)
		return err == nil
	default:
		return false
	}
}

// IsDirectory проверяет, является ли URI директорией
func IsDirectory0(uri fyne.URI) bool {
	if uri == nil {
		return false
	}
	// printFlags(uri)

	// 3. Проверка размера
	// size, sizeErr := getSize(uri)
	// log.Tracef("IsDirectory: Size: %d, error: %v", size, sizeErr)
	size, _ := getSize(uri)

	// 1. Проверка по MIME-типу
	mime := mimeType(uri)
	if mime == "vnd.android.document/directory" {
		log.Tracef("IsDirectory: Confirmed directory via MIME_TYPE: %s", mime)
		return true
	}
	if mime != "" && mime != "application/octet-stream" {
		log.Tracef("IsDirectory: Confirmed file via MIME_TYPE: %s", mime)
		return false
	}
	// log.Tracef("IsDirectory: MIME_TYPE: %s", mime)

	// 4. Специальный случай: me.zhanghai.android.files
	if size == 4096 && mime == "application/octet-stream" && strings.HasPrefix(uri.String(), ZhangHai) {
		return true
	}

	// 5. Проверка количества дочерних элементов
	// log.Tracef("IsDirectory: Checking children count...")
	count, countErr := countChild(uri)
	if countErr == nil {
		log.Tracef("IsDirectory: Confirmed directory via children count: %d", count)
		return true
	}
	log.Errorf("IsDirectory: countChild error: %v", countErr)

	// 6. Если все проверки неубедительны, предполагаем файл
	log.Tracef("IsDirectory: All checks complete, assuming file.")
	return false
}

// Если isDir == true, то childCount содержит количество дочерних элементов
// Если isDir == false, то childCount = 0
func hasChild(uri fyne.URI) (isDir bool, childCount int, err error) {
	if uri == nil {
		return false, 0, fmt.Errorf("uri is nil")
	}

	// Сначала проверяем быстрыми методами (MIME type)
	mime := mimeType(uri)
	if mime == "vnd.android.document/directory" {
		// Это точно директория - считаем детей
		isDir = true
		childCount, err = countChild(uri)
		return
	}

	if mime != "" && mime != "application/octet-stream" {
		return // Это файл
	}

	// Проверка специальных случаев
	size, sizeErr := getSize(uri)
	if sizeErr == nil && size == 4096 && mime == "application/octet-stream" &&
		strings.HasPrefix(uri.String(), ZhangHai) {
		// Специальный случай - директория
		isDir = true
		childCount, err = countChild(uri)
		return
	}

	// Последний вариант - проверка через countChild
	childCount, err = countChild(uri)
	isDir = err == nil
	return
}

// hasFlag проверяет, установлен ли определенный флаг
func hasFlag(flags int, flag int) bool {
	return flags&flag != 0
}

// printFlags печатает подробную информацию о флагах документа
func printFlags(uri fyne.URI) {
	if uri == nil {
		log.Tracef("printFlags: URI is nil")
		return
	}

	flags, err := getFlags(uri)
	if err != nil {
		log.Tracef("printFlags: Failed to get flags for %s: %v", uri, err)
		return
	}

	if flags == 0 {
		log.Tracef("printFlags: No flags available for %s", uri)
		return
	}

	log.Tracef("printFlags: Raw flags value: %d (0x%08X)", flags, flags)
	log.Tracef("printFlags: Detailed flags for %s:", uri)

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
		log.Tracef("printFlags: Set flags (%d):", len(setFlags))
		for i, flag := range setFlags {
			log.Tracef("printFlags:   [%2d] %s", i+1, flag)
		}
	} else {
		log.Tracef("printFlags: No flags set")
	}

	// Проверяем сумму установленных флагов
	if len(setFlagValues) > 0 {
		sum := 0
		for _, val := range setFlagValues {
			sum |= val
		}
		if sum == flags {
			log.Tracef("printFlags: Flag sum verification: OK (0x%08X)", sum)
		} else {
			log.Tracef("printFlags: Flag sum verification: MISMATCH! Calculated: 0x%08X, Actual: 0x%08X", sum, flags)
			log.Tracef("printFlags: There might be unknown flags set: 0x%08X", flags^sum)
		}
	}

	// Информация о правах доступа
	writeAccess := flags&0x00000002 != 0
	deleteAccess := flags&0x00000001 != 0
	readAccess := flags&0x00000002 != 0 // Обычно WRITE подразумевает и READ

	log.Tracef("printFlags: Access - Read: %v, Write: %v, Delete: %v",
		readAccess, writeAccess, deleteAccess)
}
