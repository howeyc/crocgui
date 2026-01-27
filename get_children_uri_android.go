//go:build android

// get_children_uri_android.go
// func list(uri fyne.URI) (children []fyne.URI, err error) {return nil, nil}
package main

/*
#include <jni.h>
#include <string.h>
#include <stdlib.h>
#include <android/log.h>

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

static char* getChildrenURI(JNIEnv* env, jobject activity, const char* uriStr) {
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

    char* result = NULL;
    char* currentBuffer = NULL;
    size_t bufferSize = 4096; // Еще больше для URI
    size_t currentLength = 0;

    // Инициализация ContentResolver - ТОЧНАЯ КОПИЯ из getChildrenList
    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("getChildrenURI: Failed to get activity class");
        goto cleanup;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("getChildrenURI: Failed to get getContentResolver method");
        goto cleanup;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) {
        LogD("getChildrenURI: contentResolver is NULL or exception occurred");
        goto cleanup;
    }

    // Парсинг URI - ТОЧНАЯ КОПИЯ из getChildrenList
    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("getChildrenURI: Failed to find Uri class");
        goto cleanup;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("getChildrenURI: Failed to get parse method");
        goto cleanup;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) {
        LogD("getChildrenURI: Failed to parse URI");
        goto cleanup;
    }

    documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogD("getChildrenURI: Failed to find DocumentsContract class");
        goto cleanup;
    }

    // ПОДХОД 1: Прямой query с разными вариантами - ТОЧНАЯ КОПИЯ из getChildrenList
    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        LogD("getChildrenURI: Failed to get resolver class");
        goto cleanup;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass,
        "query", "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("getChildrenURI: Failed to get query method");
        goto cleanup;
    }

    // Сначала пробуем стандартный подход с buildChildDocumentsUriUsingTree
    jmethodID buildChildDocumentsUriUsingTreeMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
    jmethodID getTreeDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass, "getTreeDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");

    if (buildChildDocumentsUriUsingTreeMethod != NULL && getTreeDocumentIdMethod != NULL) {
        jstring treeDocId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getTreeDocumentIdMethod, uri);
        if (!caseException(env, "getTreeDocumentId") && treeDocId != NULL) {
            childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                buildChildDocumentsUriUsingTreeMethod, uri, treeDocId);
            if (!caseException(env, "buildChildDocumentsUriUsingTree") && childUri != NULL) {
                LogD("getChildrenURI: Successfully built child URI using tree method");

                // Пробуем query с этим URI
                jclass stringClass = (*env)->FindClass(env, "java/lang/String");
                if (stringClass != NULL) {
                    jobjectArray projection = (*env)->NewObjectArray(env, 2, stringClass, NULL);
                    if (projection != NULL) {
                        jstring colDisplayName = (*env)->NewStringUTF(env, "_display_name");
                        jstring colDocumentId = (*env)->NewStringUTF(env, "document_id");

                        (*env)->SetObjectArrayElement(env, projection, 0, colDisplayName);
                        (*env)->SetObjectArrayElement(env, projection, 1, colDocumentId);

                        cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, projection, NULL, NULL, NULL);

                        (*env)->DeleteLocalRef(env, colDisplayName);
                        (*env)->DeleteLocalRef(env, colDocumentId);
                        (*env)->DeleteLocalRef(env, projection);
                    }
                    (*env)->DeleteLocalRef(env, stringClass);
                }

                // Если не сработало, пробуем без проекции
                if (caseException(env, "query with projection") || cursor == NULL) {
                    if (cursor != NULL) {
                        (*env)->DeleteLocalRef(env, cursor);
                        cursor = NULL;
                    }
                    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, NULL, NULL, NULL, NULL);
                }
            }
            (*env)->DeleteLocalRef(env, treeDocId);
        }
    }

    // ПОДХОД 2: Если стандартный подход не сработал, используем альтернативный метод
    if (caseException(env, "standard approach") || cursor == NULL) {
        LogD("getChildrenURI: Standard approach failed, using alternative method");

        if (cursor != NULL) {
            (*env)->DeleteLocalRef(env, cursor);
            cursor = NULL;
        }

        // Используем альтернативный метод построения childUri
        jmethodID getDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass, "getDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");

        if (getDocumentIdMethod != NULL && buildChildDocumentsUriUsingTreeMethod != NULL) {
            jstring docId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getDocumentIdMethod, uri);
            if (!caseException(env, "getDocumentId for fallback") && docId != NULL) {
                // Освобождаем предыдущий childUri если был
                if (childUri != NULL) {
                    (*env)->DeleteLocalRef(env, childUri);
                    childUri = NULL;
                }

                childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                    buildChildDocumentsUriUsingTreeMethod, uri, docId);
                if (!caseException(env, "buildChildDocumentsUriUsingTree fallback") && childUri != NULL) {
                    LogD("getChildrenURI: Successfully built child URI using fallback tree method");

                    // Проекция только для document_id (минимальная)
                    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
                    if (stringClass != NULL) {
                        jobjectArray simpleProjection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
                        if (simpleProjection != NULL) {
                            jstring colDocIdOnly = (*env)->NewStringUTF(env, "document_id");
                            (*env)->SetObjectArrayElement(env, simpleProjection, 0, colDocIdOnly);

                            cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, simpleProjection, NULL, NULL, NULL);

                            (*env)->DeleteLocalRef(env, colDocIdOnly);
                            (*env)->DeleteLocalRef(env, simpleProjection);
                        }
                        (*env)->DeleteLocalRef(env, stringClass);
                    }
                }
                (*env)->DeleteLocalRef(env, docId);
            }
        }
    }

    // ПОДХОД 3: Если все еще не сработало, пробуем getChildrenDocuments
    if ((caseException(env, "fallback approach") || cursor == NULL) && documentsContractClass != NULL) {
        LogD("getChildrenURI: Trying getChildrenDocuments as last resort");

        if (cursor != NULL) {
            (*env)->DeleteLocalRef(env, cursor);
            cursor = NULL;
        }

        jmethodID getChildrenDocumentsMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
            "getChildrenDocuments", "(Landroid/content/ContentResolver;Landroid/net/Uri;)Landroid/database/Cursor;");

        if (getChildrenDocumentsMethod != NULL) {
            cursor = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                getChildrenDocumentsMethod, contentResolver, uri);
            if (caseException(env, "getChildrenDocuments")) {
                LogD("getChildrenURI: getChildrenDocuments failed with exception");
            }
        }
    }

    // Если ни один метод не сработал, возвращаем ошибку
    if (caseException(env, "all query methods") || cursor == NULL) {
        LogD("getChildrenURI: All methods failed to get cursor");
        goto cleanup;
    }

    // Обработка курсора - ИЗМЕНЕННАЯ ЧАСТЬ: строим полные URI вместо имен
    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        LogD("getChildrenURI: Failed to get cursor class");
        goto cleanup;
    }

    jmethodID getCount = (*env)->GetMethodID(env, cursorClass, "getCount", "()I");
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID moveToNext = (*env)->GetMethodID(env, cursorClass, "moveToNext", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");
    jmethodID getColumnIndex = (*env)->GetMethodID(env, cursorClass, "getColumnIndex", "(Ljava/lang/String;)I");

    if (getCount == NULL || moveToFirst == NULL || moveToNext == NULL || getString == NULL || getColumnIndex == NULL) {
        LogD("getChildrenURI: Failed to get cursor methods");
        goto cleanup;
    }

    jint count = (*env)->CallIntMethod(env, cursor, getCount);
    if (caseException(env, "getCount in children list")) {
        LogD("getChildrenURI: getCount failed with exception");
        goto cleanup;
    }

    LogD("getChildrenURI: Found %d children", count);

    if (count <= 0) {
        result = strdup("");
        goto cleanup;
    }

    // Получаем индекс столбца с document_id - ОСНОВНОЙ СТОЛБЕЦ ДЛЯ URI
    jstring documentIdColumn = (*env)->NewStringUTF(env, "document_id");
    jint colDocumentIdIndex = (*env)->CallIntMethod(env, cursor, getColumnIndex, documentIdColumn);
    (*env)->DeleteLocalRef(env, documentIdColumn);

    if (colDocumentIdIndex < 0) {
        LogD("getChildrenURI: document_id column not found");
        goto cleanup;
    }

    LogD("getChildrenURI: Using column index %d for document_id", colDocumentIdIndex);

    // Получаем метод toString для Uri
    jmethodID uriToStringMethod = (*env)->GetMethodID(env, uriClass, "toString", "()Ljava/lang/String;");
    if (uriToStringMethod == NULL) {
        LogD("getChildrenURI: Failed to get Uri.toString method");
        goto cleanup;
    }

    // Получаем методы для построения URI дочерних документов
    jmethodID buildDocumentUriUsingTreeMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildDocumentUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");

    // Инициализация буфера
    currentBuffer = malloc(bufferSize);
    if (currentBuffer == NULL) {
        LogD("getChildrenURI: Failed to allocate buffer");
        goto cleanup;
    }
    currentBuffer[0] = '\0';

    // Читаем данные из курсора и строим полные URI для каждого ребенка
    jboolean hasData = (*env)->CallBooleanMethod(env, cursor, moveToFirst);
    jint index = 0;
    jboolean firstItem = JNI_TRUE;

    while (hasData && index < count) {
        jstring childDocumentId = (jstring)(*env)->CallObjectMethod(env, cursor, getString, colDocumentIdIndex);

        if (childDocumentId != NULL) {
            const char* docIdStr = (*env)->GetStringUTFChars(env, childDocumentId, NULL);
            if (docIdStr != NULL) {
                // Строим полный URI для дочернего документа
                jobject childDocumentUri = NULL;
                jstring childUriString = NULL;
                const char* uriStrValue = NULL;

                // Пробуем buildDocumentUriUsingTree сначала
                if (buildDocumentUriUsingTreeMethod != NULL) {
                    childDocumentUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                        buildDocumentUriUsingTreeMethod, uri, childDocumentId);
                    if (caseException(env, "buildDocumentUriUsingTree")) {
                        childDocumentUri = NULL;
                    }
                }

                // Если не сработало, пробуем buildChildDocumentsUriUsingTree как fallback
                if (childDocumentUri == NULL && buildChildDocumentsUriUsingTreeMethod != NULL) {
                    childDocumentUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                        buildChildDocumentsUriUsingTreeMethod, uri, childDocumentId);
                    if (caseException(env, "buildChildDocumentsUriUsingTree for child")) {
                        childDocumentUri = NULL;
                    }
                }

                // Если оба метода не сработали, создаем URI вручную из document_id
                if (childDocumentUri == NULL) {
                    // Пытаемся создать URI вручную на основе оригинального URI и document_id
                    jmethodID buildUponMethod = (*env)->GetMethodID(env, uriClass, "buildUpon", "()Landroid/net/Uri$Builder;");
                    if (buildUponMethod != NULL) {
                        jobject uriBuilder = (*env)->CallObjectMethod(env, uri, buildUponMethod);
                        if (!caseException(env, "buildUpon") && uriBuilder != NULL) {
                            jclass uriBuilderClass = (*env)->GetObjectClass(env, uriBuilder);
                            if (uriBuilderClass != NULL) {
                                jmethodID appendPathMethod = (*env)->GetMethodID(env, uriBuilderClass, "appendPath", "(Ljava/lang/String;)Landroid/net/Uri$Builder;");
                                if (appendPathMethod != NULL) {
                                    // Добавляем document_id как путь
                                    jobject newBuilder = (*env)->CallObjectMethod(env, uriBuilder, appendPathMethod, childDocumentId);
                                    if (!caseException(env, "appendPath")) {
                                        jmethodID buildMethod = (*env)->GetMethodID(env, uriBuilderClass, "build", "()Landroid/net/Uri;");
                                        if (buildMethod != NULL) {
                                            childDocumentUri = (*env)->CallObjectMethod(env, newBuilder, buildMethod);
                                        }
                                    }
                                }
                                (*env)->DeleteLocalRef(env, uriBuilderClass);
                            }
                            (*env)->DeleteLocalRef(env, uriBuilder);
                        }
                    }
                }

                if (childDocumentUri != NULL) {
                    // Получаем строковое представление URI
                    childUriString = (jstring)(*env)->CallObjectMethod(env, childDocumentUri, uriToStringMethod);
                    if (childUriString != NULL) {
                        uriStrValue = (*env)->GetStringUTFChars(env, childUriString, NULL);
                    }
                    (*env)->DeleteLocalRef(env, childDocumentUri);
                }

                // Если удалось получить URI, добавляем его в буфер
                if (uriStrValue != NULL) {
                    size_t uriLen = strlen(uriStrValue);

                    // Проверяем, достаточно ли места в буфере
                    if (currentLength + uriLen + 2 > bufferSize) {
                        bufferSize = (currentLength + uriLen + 2) * 2;
                        char* newBuffer = realloc(currentBuffer, bufferSize);
                        if (newBuffer == NULL) {
                            (*env)->ReleaseStringUTFChars(env, childUriString, uriStrValue);
                            (*env)->DeleteLocalRef(env, childUriString);
                            (*env)->ReleaseStringUTFChars(env, childDocumentId, docIdStr);
                            (*env)->DeleteLocalRef(env, childDocumentId);
                            LogD("getChildrenURI: Failed to reallocate buffer");
                            goto cleanup;
                        }
                        currentBuffer = newBuffer;
                    }

                    // Добавляем разделитель если это не первый элемент
                    if (!firstItem) {
                        strcat(currentBuffer, "|");
                        currentLength++;
                    } else {
                        firstItem = JNI_FALSE;
                    }

                    // Добавляем URI
                    strcat(currentBuffer, uriStrValue);
                    currentLength += uriLen;

                    LogD("getChildrenURI: Added child URI: %s", uriStrValue);

                    (*env)->ReleaseStringUTFChars(env, childUriString, uriStrValue);
                } else {
                    // Если не удалось построить URI, используем document_id как fallback
                    LogD("getChildrenURI: Failed to build URI for document_id: %s", docIdStr);
                }

                if (childUriString != NULL) {
                    (*env)->DeleteLocalRef(env, childUriString);
                }
                (*env)->ReleaseStringUTFChars(env, childDocumentId, docIdStr);
            }
            (*env)->DeleteLocalRef(env, childDocumentId);
        }

        index++;
        hasData = (*env)->CallBooleanMethod(env, cursor, moveToNext);
    }

    if (currentLength > 0) {
        result = strdup(currentBuffer);
        LogD("getChildrenURI: Successfully built %d child URIs", index);
    } else {
        result = strdup("");
        LogD("getChildrenURI: No child URIs built");
    }

cleanup:
    if (currentBuffer != NULL) {
        free(currentBuffer);
    }

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

    if (childUri != NULL) {
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

    return result;
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

func list(uri fyne.URI) (children []fyne.URI, err error) {
	if uri == nil {
		return nil, fmt.Errorf("uri is nil")
	}

	var childrenStr string

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		// Вызываем C функцию, которая возвращает строку с URI через разделитель |
		cChildren := C.getChildrenURI(env, activity, uriStr)
		defer C.free(unsafe.Pointer(cChildren))

		if cChildren == nil {
			err = fmt.Errorf("failed to get children URI: returned NULL")
			return nil
		}

		childrenStr = C.GoString(cChildren)

		if childrenStr == "" {
			// Пустой каталог - не ошибка, возвращаем пустой слайс
			children = []fyne.URI{}
		} else {
			log.Debugf("getChildrenURI: successfully got children URI string for URI: %s", uri.String())
		}

		return nil
	})

	// Если была ошибка, возвращаем ее
	if err != nil {
		return nil, err
	}

	// Если строка пустая, возвращаем пустой слайс
	if childrenStr == "" {
		return []fyne.URI{}, nil
	}

	// Разбиваем строку по разделителю | на слайс строк
	uriStrs := strings.Split(childrenStr, "|")
	children = make([]fyne.URI, 0, len(uriStrs))

	// Конвертируем каждую строку в fyne.URI
	for _, uriStr := range uriStrs {
		if uriStr != "" {
			childURI := storage.NewURI(uriStr)
			children = append(children, childURI)
		}
	}

	log.Debugf("getChildrenURI: parsed %d children URIs for parent URI: %s", len(children), uri.String())

	return children, nil
}
func List(u fyne.URI) (c []fyne.URI, err error) {
	if u == nil {
		err = fmt.Errorf("uri is nul")
		return
	}
	if u.Scheme() == "content" {
		return list(u)
	}
	return storage.List(u)
}
