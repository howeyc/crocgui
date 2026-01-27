//go:build android

// get_children_list_android.go
// func getChildrenList(uri fyne.URI) (children []string, err error) {return nil, nil}
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

static char* getChildrenList(JNIEnv* env, jobject activity, const char* uriStr) {
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
    size_t bufferSize = 2048;
    size_t currentLength = 0;

    // Инициализация ContentResolver
    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("getChildrenList: Failed to get activity class");
        goto cleanup;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolver == NULL) {
        LogD("getChildrenList: Failed to get getContentResolver method");
        goto cleanup;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (caseException(env, "getContentResolver") || contentResolver == NULL) {
        LogD("getChildrenList: contentResolver is NULL or exception occurred");
        goto cleanup;
    }

    // Парсинг URI
    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        LogD("getChildrenList: Failed to find Uri class");
        goto cleanup;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        LogD("getChildrenList: Failed to get parse method");
        goto cleanup;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (caseException(env, "parse URI") || uri == NULL) {
        LogD("getChildrenList: Failed to parse URI");
        goto cleanup;
    }

    documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogD("getChildrenList: Failed to find DocumentsContract class");
        goto cleanup;
    }

    // ПОДХОД 1: Прямой query с разными вариантами (работает для стандартных провайдеров)
    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        LogD("getChildrenList: Failed to get resolver class");
        goto cleanup;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass,
        "query", "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        LogD("getChildrenList: Failed to get query method");
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
                LogD("getChildrenList: Successfully built child URI using tree method");

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

    // ПОДХОД 2: Если стандартный подход не сработал, используем логику из getChildrenURI
    if (caseException(env, "standard approach") || cursor == NULL) {
        LogD("getChildrenList: Standard approach failed, using getChildrenURI logic");

        if (cursor != NULL) {
            (*env)->DeleteLocalRef(env, cursor);
            cursor = NULL;
        }

        // Используем альтернативный метод построения childUri (который работает в getChildrenURI)
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
                    LogD("getChildrenList: Successfully built child URI using fallback tree method");

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
        LogD("getChildrenList: Trying getChildrenDocuments as last resort");

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
                LogD("getChildrenList: getChildrenDocuments failed with exception");
            }
        }
    }

    // Если ни один метод не сработал, возвращаем ошибку
    if (caseException(env, "all query methods") || cursor == NULL) {
        LogD("getChildrenList: All methods failed to get cursor");
        goto cleanup;
    }

    // Обработка курсора - извлекаем имена файлов
    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        LogD("getChildrenList: Failed to get cursor class");
        goto cleanup;
    }

    jmethodID getCount = (*env)->GetMethodID(env, cursorClass, "getCount", "()I");
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID moveToNext = (*env)->GetMethodID(env, cursorClass, "moveToNext", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");
    jmethodID getColumnIndex = (*env)->GetMethodID(env, cursorClass, "getColumnIndex", "(Ljava/lang/String;)I");

    if (getCount == NULL || moveToFirst == NULL || moveToNext == NULL || getString == NULL || getColumnIndex == NULL) {
        LogD("getChildrenList: Failed to get cursor methods");
        goto cleanup;
    }

    jint count = (*env)->CallIntMethod(env, cursor, getCount);
    if (caseException(env, "getCount in children list")) {
        LogD("getChildrenList: getCount failed with exception");
        goto cleanup;
    }

    LogD("getChildrenList: Found %d children", count);

    if (count <= 0) {
        result = strdup("");
        goto cleanup;
    }

    // Получаем индекс столбца с именами - пробуем разные варианты
    jint nameColumnIndex = -1;

    // Сначала пробуем _display_name
    jstring displayNameColumn = (*env)->NewStringUTF(env, "_display_name");
    nameColumnIndex = (*env)->CallIntMethod(env, cursor, getColumnIndex, displayNameColumn);
    (*env)->DeleteLocalRef(env, displayNameColumn);

    // Если не нашли, пробуем document_id (будет извлекать имена из путей)
    if (nameColumnIndex < 0) {
        jstring documentIdColumn = (*env)->NewStringUTF(env, "document_id");
        nameColumnIndex = (*env)->CallIntMethod(env, cursor, getColumnIndex, documentIdColumn);
        (*env)->DeleteLocalRef(env, documentIdColumn);

        if (nameColumnIndex >= 0) {
            LogD("getChildrenList: Using document_id column - will extract names from paths");
        }
    }

    // Если все еще не нашли, пробуем другие возможные столбцы
    if (nameColumnIndex < 0) {
        const char* possibleColumns[] = {"display_name", "name", "title", "file_name", NULL};
        for (int i = 0; possibleColumns[i] != NULL; i++) {
            jstring columnName = (*env)->NewStringUTF(env, possibleColumns[i]);
            nameColumnIndex = (*env)->CallIntMethod(env, cursor, getColumnIndex, columnName);
            (*env)->DeleteLocalRef(env, columnName);
            if (nameColumnIndex >= 0) {
                LogD("getChildrenList: Found name column at index %d for column '%s'", nameColumnIndex, possibleColumns[i]);
                break;
            }
        }
    }

    if (nameColumnIndex < 0) {
        LogD("getChildrenList: No valid name column found, using first column as fallback");
        nameColumnIndex = 0;
    }

    LogD("getChildrenList: Using column index %d for names", nameColumnIndex);

    // Инициализация буфера
    currentBuffer = malloc(bufferSize);
    if (currentBuffer == NULL) {
        LogD("getChildrenList: Failed to allocate buffer");
        goto cleanup;
    }
    currentBuffer[0] = '\0';

    // Читаем данные из курсора
    jboolean hasData = (*env)->CallBooleanMethod(env, cursor, moveToFirst);
    jint index = 0;
    jboolean firstItem = JNI_TRUE;

    while (hasData && index < count) {
        jstring itemName = (jstring)(*env)->CallObjectMethod(env, cursor, getString, nameColumnIndex);

        if (itemName != NULL) {
            const char* nameStr = (*env)->GetStringUTFChars(env, itemName, NULL);
            if (nameStr != NULL) {
                // Если используем document_id, может потребоваться извлечь имя файла из пути
                const char* finalName = nameStr;

                // Проверяем, является ли это путем (содержит '/')
                if (strchr(nameStr, '/') != NULL) {
                    // Извлекаем последний компонент пути как имя файла
                    const char* lastSlash = strrchr(nameStr, '/');
                    if (lastSlash != NULL) {
                        finalName = lastSlash + 1;
                    }
                }

                size_t nameLen = strlen(finalName);

                // Проверяем, достаточно ли места в буфере
                if (currentLength + nameLen + 2 > bufferSize) {
                    bufferSize = (currentLength + nameLen + 2) * 2;
                    char* newBuffer = realloc(currentBuffer, bufferSize);
                    if (newBuffer == NULL) {
                        (*env)->ReleaseStringUTFChars(env, itemName, nameStr);
                        (*env)->DeleteLocalRef(env, itemName);
                        LogD("getChildrenList: Failed to reallocate buffer");
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

                // Добавляем имя файла
                strcat(currentBuffer, finalName);
                currentLength += nameLen;

                LogD("getChildrenList: Added item: %s", finalName);

                (*env)->ReleaseStringUTFChars(env, itemName, nameStr);
            }
            (*env)->DeleteLocalRef(env, itemName);
        }

        index++;
        hasData = (*env)->CallBooleanMethod(env, cursor, moveToNext);
    }

    if (currentLength > 0) {
        result = strdup(currentBuffer);
        LogD("getChildrenList: Successfully read %d items, result: %s", index, result);
    } else {
        result = strdup("");
        LogD("getChildrenList: No items read");
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
	log "github.com/schollz/logger"
)

func getChildrenList(uri fyne.URI) (children []string, err error) {
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

		// Вызываем C функцию, которая возвращает строку с разделителем |
		cChildren := C.getChildrenList(env, activity, uriStr)
		defer C.free(unsafe.Pointer(cChildren))

		if cChildren == nil {
			err = fmt.Errorf("failed to get children list: returned NULL")
			return nil
		}

		childrenStr = C.GoString(cChildren)

		if childrenStr == "" {
			err = fmt.Errorf("no children found or empty directory")
		} else {
			log.Debugf("getChildrenList: successfully got children string for URI: %s", uri.String())
		}

		return nil
	})

	// Если была ошибка, возвращаем ее
	if err != nil {
		return nil, err
	}

	// Если строка пустая, возвращаем пустой слайс
	if childrenStr == "" {
		return []string{}, nil
	}

	// Разбиваем строку по разделителю | на слайс строк
	children = strings.Split(childrenStr, "|")

	log.Debugf("getChildrenList: parsed %d children for URI: %s", len(children), uri.String())

	return children, nil
}
