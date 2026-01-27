//go:build android

// count_children_android.go
// func countChild(uri fyne.URI) (count int, err error) {return 0, nil}
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
    if (caseException(env, "getContentResolver") || contentResolver == NULL) {
        LogD("countChildren: contentResolver is NULL or exception occurred");
        goto cleanup;
    }

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
    if (caseException(env, "parse URI") || uri == NULL) {
        LogD("countChildren: Failed to parse URI");
        goto cleanup;
    }

    documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogD("countChildren: Failed to find DocumentsContract class");
        goto cleanup;
    }

    // Метод 1: buildChildDocumentsUriUsingTree (основной)
    jmethodID buildChildDocumentsUriUsingTreeMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");

    if (buildChildDocumentsUriUsingTreeMethod != NULL) {
        jmethodID getTreeDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass, "getTreeDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");
        if (getTreeDocumentIdMethod != NULL) {
            jstring treeDocId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getTreeDocumentIdMethod, uri);
            if (caseException(env, "getTreeDocumentId")) {
                // Было исключение - переходим к следующему методу
                LogD("countChildren: getTreeDocumentId failed with exception");
            } else if (treeDocId != NULL) {
                childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                    buildChildDocumentsUriUsingTreeMethod, uri, treeDocId);
                if (caseException(env, "buildChildDocumentsUriUsingTree")) {
                    // Было исключение - очищаем и продолжаем
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
    }

    // Метод 2: buildChildDocumentsUri (альтернативный)
    if (childUri == NULL) {
        jmethodID buildChildDocumentsUriMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
            "buildChildDocumentsUri", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");

        if (buildChildDocumentsUriMethod != NULL) {
            jmethodID getDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass, "getDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");
            if (getDocumentIdMethod != NULL) {
                jstring docId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getDocumentIdMethod, uri);
                if (caseException(env, "getDocumentId")) {
                    // Было исключение - переходим к следующему методу
                    LogD("countChildren: getDocumentId failed with exception");
                } else if (docId != NULL) {
                    childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                        buildChildDocumentsUriMethod, uri, docId);
                    if (caseException(env, "buildChildDocumentsUri")) {
                        // Было исключение - очищаем и продолжаем
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
        }
    }

    // Метод 3: Прямой query исходного URI (последняя попытка)
    if (childUri == NULL) {
        LogD("countChildren: Using direct URI query as fallback");
        childUri = (*env)->NewLocalRef(env, uri); // Безопасное создание новой ссылки
        childUriNeedsCleanup = JNI_TRUE;
    }

    if (childUri != NULL) {
        resolverClass = (*env)->GetObjectClass(env, contentResolver);
        if (resolverClass == NULL) {
            LogD("countChildren: Failed to get resolver class");
            goto cleanup;
        }

        jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass,
            "query", "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");

        if (queryMethod != NULL) {
            cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, NULL, NULL, NULL, NULL);
            if (caseException(env, "query for children")) {
                LogD("countChildren: Query failed with exception");
                count = -8; // Код ошибки запроса
            } else if (cursor != NULL) {
                cursorClass = (*env)->GetObjectClass(env, cursor);
                if (cursorClass != NULL) {
                    jmethodID getCount = (*env)->GetMethodID(env, cursorClass, "getCount", "()I");

                    if (getCount != NULL) {
                        count = (*env)->CallIntMethod(env, cursor, getCount);
                        if (caseException(env, "getCount")) {
                            LogD("countChildren: getCount failed with exception");
                            count = -9; // Код ошибки getCount
                        } else {
                            LogD("countChildren: Successfully got count: %d", count);
                        }
                    } else {
                        LogD("countChildren: Failed to get getCount method");
                        count = -3; // Код ошибки методов курсора
                    }

                    // Закрываем курсор
                    jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
                    if (closeMethod != NULL) {
                        (*env)->CallVoidMethod(env, cursor, closeMethod);
                        caseException(env, "close cursor");
                    }
                } else {
                    LogD("countChildren: Failed to get cursor class");
                    count = -4; // Код ошибки класса курсора
                }
            } else {
                LogD("countChildren: Query returned NULL cursor");
                count = -5; // Код ошибки курсора
            }
        } else {
            LogD("countChildren: Failed to get query method");
            count = -6; // Код ошибки метода запроса
        }
    } else {
        LogD("countChildren: Failed to build child URI");
        count = -7; // Код ошибки построения URI
    }

cleanup:
    // Освобождаем ресурсы в правильном порядке
    if (cursor != NULL) {
        // Дополнительная попытка закрыть курсор, если не закрыт ранее
        if (cursorClass != NULL) {
            jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
            if (closeMethod != NULL) {
                (*env)->CallVoidMethod(env, cursor, closeMethod);
                caseException(env, "close cursor in cleanup");
            }
        }
        (*env)->DeleteLocalRef(env, cursor);
    }

    // Аккуратно управляем childUri
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
	"os"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

// countChild возвращает количество дочерних элементов в директории DocumentsContract
func countChild(uri fyne.URI) (count int, err error) {
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
			log.Debugf("countChildren: successfully counted %d children for URI: %s", count, uri.String())
		}

		return nil
	})

	return count, err
}

// IsDirectory проверяет, является ли URI каталогом
func IsDirectory(uri fyne.URI) bool {
	// log.Debugf("-------------IsDirectory %s", uri)
	if uri == nil {
		return false
	}
	if uri.Scheme() == "file" {
		if fi, err := os.Stat(uri.Path()); err == nil {
			ok := fi.IsDir()
			// log.Debugf("-------------IsDir() %v", ok)
			return ok
		}
	}
	switch MimeType(uri) {
	case MIME_TYPE_DIR:
		// log.Debug("-------------MIME_TYPE_DIR true")
		return true
	case MIME_TYPE_OCTET_STREAM:
		if strings.HasPrefix(uri.String(), ZhangHai) {
			size, sizeErr := getSize(uri)
			if sizeErr == nil && size == 4096 {
				// log.Debug("-------------ZhangHai true")
				return true
			}
		}
		fallthrough
	case "":
		// отсутствуют права
		_, err := countChild(uri)
		// log.Debugf("-------------countChild %d error: %v %v", c, err, err == nil)
		return err == nil
	default:
		// log.Debug("-------------false")
		return false
	}
}
