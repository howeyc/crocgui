//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <android/log.h>

#define LOG_TAG "croc"
#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, LOG_TAG, __VA_ARGS__)
#define LogE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)

// Структура для хранения путей файлов
typedef struct {
    char* srcURI;
    char* dstPath;
} FileCopyTask;

// Глобальная переменная для хранения оригинального tree URI
static const char* g_original_tree_uri = NULL;

// Проверка исключений и логирование ошибок
static jboolean CheckException(JNIEnv* env, const char* context) {
    if ((*env)->ExceptionCheck(env)) {
        LogE("Exception in %s", context);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE;
    }
    return JNI_FALSE;
}

// Безопасное выделение памяти
static void* SafeMalloc(size_t size, const char* context) {
    void* ptr = malloc(size);
    if (ptr == NULL) {
        LogE("OutOfMemoryError in %s: failed to allocate %zu bytes", context, size);
    }
    return ptr;
}

static char* SafeStrdup(const char* str, const char* context) {
    if (str == NULL) return NULL;
    char* new_str = strdup(str);
    if (new_str == NULL) {
        LogE("OutOfMemoryError in %s: failed to duplicate string", context);
    }
    return new_str;
}

// Получение ContentResolver
static jobject GetContentResolver(JNIEnv* env, jobject activity) {
    jclass activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) return NULL;

    jmethodID getContentResolverMethod = (*env)->GetMethodID(env, activityClass,
        "getContentResolver", "()Landroid/content/ContentResolver;");
    if (getContentResolverMethod == NULL) {
        (*env)->DeleteLocalRef(env, activityClass);
        return NULL;
    }

    jobject contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolverMethod);
    (*env)->DeleteLocalRef(env, activityClass);

    return contentResolver;
}

// Парсинг URI
static jobject ParseUri(JNIEnv* env, const char* uriStr) {
    jclass uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) return NULL;

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass,
        "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (parseMethod == NULL) {
        (*env)->DeleteLocalRef(env, uriClass);
        return NULL;
    }

    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);
    (*env)->DeleteLocalRef(env, uriClass);

    return uri;
}

// Получение documentId из tree URI
static char* GetTreeDocumentId(JNIEnv* env, jobject activity, const char* treeUriStr) {
    jobject treeUri = ParseUri(env, treeUriStr);
    if (treeUri == NULL) {
        LogE("Failed to parse tree URI: %s", treeUriStr);
        return NULL;
    }

    jclass documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogE("Failed to find DocumentsContract class");
        (*env)->DeleteLocalRef(env, treeUri);
        return NULL;
    }

    jmethodID getTreeDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "getTreeDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");
    if (getTreeDocumentIdMethod == NULL) {
        LogE("Failed to get getTreeDocumentId method");
        (*env)->DeleteLocalRef(env, documentsContractClass);
        (*env)->DeleteLocalRef(env, treeUri);
        return NULL;
    }

    jstring jdocumentId = (*env)->CallStaticObjectMethod(env, documentsContractClass,
        getTreeDocumentIdMethod, treeUri);

    (*env)->DeleteLocalRef(env, documentsContractClass);
    (*env)->DeleteLocalRef(env, treeUri);

    if (jdocumentId == NULL) {
        LogE("getTreeDocumentId returned NULL");
        return NULL;
    }

    const char* documentIdStr = (*env)->GetStringUTFChars(env, jdocumentId, NULL);
    char* result = SafeStrdup(documentIdStr, "GetTreeDocumentId");
    (*env)->ReleaseStringUTFChars(env, jdocumentId, documentIdStr);
    (*env)->DeleteLocalRef(env, jdocumentId);

    LogD("Tree Document ID (root): %s", result);
    return result;
}

// Получение documentId для произвольного document URI
static char* GetDocumentId(JNIEnv* env, jobject documentUriObj) {
    jclass documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogE("Failed to find DocumentsContract class in GetDocumentId");
        return NULL;
    }

    jmethodID getDocumentIdMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "getDocumentId", "(Landroid/net/Uri;)Ljava/lang/String;");
    if (getDocumentIdMethod == NULL) {
        LogE("Failed to get getDocumentId method");
        (*env)->DeleteLocalRef(env, documentsContractClass);
        return NULL;
    }

    jstring jdocumentId = (*env)->CallStaticObjectMethod(env, documentsContractClass,
        getDocumentIdMethod, documentUriObj);

    (*env)->DeleteLocalRef(env, documentsContractClass);

    if (jdocumentId == NULL) {
        LogE("getDocumentId returned NULL");
        return NULL;
    }

    const char* documentIdStr = (*env)->GetStringUTFChars(env, jdocumentId, NULL);
    char* result = SafeStrdup(documentIdStr, "GetDocumentId-result");
    (*env)->ReleaseStringUTFChars(env, jdocumentId, documentIdStr);
    (*env)->DeleteLocalRef(env, jdocumentId);

    LogD("Got Document ID: %s", result);
    return result;
}

// Построение document URI из *original* tree URI и *documentId* (ручное построение)
static char* BuildDocumentUri(JNIEnv* env, jobject activity, const char* originalTreeUri, const char* documentId) {
    // Находим начало authority: "content://"
    const char* prefix = "content://";
    if (strncmp(originalTreeUri, prefix, strlen(prefix)) != 0) {
        LogE("Not a content URI: %s", originalTreeUri);
        return NULL;
    }
    const char* after_prefix = originalTreeUri + strlen(prefix);

    // Находим конец authority (первый '/' после "content://" или конец строки)
    const char* authority_end = strchr(after_prefix, '/');
    if (authority_end == NULL) {
        // Если нет '/', вся оставшаяся часть - authority (маловероятно, но на всякий случай)
        authority_end = originalTreeUri + strlen(originalTreeUri);
    }

    size_t authority_len = authority_end - after_prefix;
    size_t documentId_len = strlen(documentId);

    // document_uri = "content://" + authority + "/document/" + documentId

    size_t result_len = strlen(prefix) + authority_len + strlen("/document/") + documentId_len + 1;
    char* result = SafeMalloc(result_len, "BuildDocumentUri-manual");
    if (result == NULL) {
        return NULL;
    }

    snprintf(result, result_len, "%.*s%.*s%s%s", (int)strlen(prefix), prefix, (int)authority_len, after_prefix, "/document/", documentId);

    LogD("Built document URI (manual): %s", result);
    return result;
}

// Получение document URI для корневой директории tree URI
static char* GetRootDocumentUri(JNIEnv* env, jobject activity, const char* treeUriStr) {
    char* documentId = GetTreeDocumentId(env, activity, treeUriStr);
    if (documentId == NULL) {
        LogE("Failed to get document ID for tree URI");
        return NULL;
    }

    char* documentUri = BuildDocumentUri(env, activity, treeUriStr, documentId);
    free(documentId);

    return documentUri;
}

// Получение информации о файле (имя и MIME тип)
static char* GetFileInfo(JNIEnv* env, jobject activity, const char* uriStr, const char* column) {
    LogD("GetFileInfo for URI: %s, column: %s", uriStr, column);

    jobject contentResolver = GetContentResolver(env, activity);
    if (contentResolver == NULL) {
        LogE("ContentResolver is NULL");
        return NULL;
    }

    jobject uri = ParseUri(env, uriStr);
    if (uri == NULL) {
        LogE("Failed to parse URI: %s", uriStr);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");

    if (queryMethod == NULL) {
        LogE("Failed to get query method");
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    jstring columnName = (*env)->NewStringUTF(env, column);
    (*env)->SetObjectArrayElement(env, projection, 0, columnName);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Очистка временных ссылок
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, columnName);
    (*env)->DeleteLocalRef(env, stringClass);
    (*env)->DeleteLocalRef(env, resolverClass);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, contentResolver);

    if (cursor == NULL) {
        LogE("Cursor is NULL for query");
        return NULL;
    }

    char* result = NULL;
    jclass cursorClass = (*env)->GetObjectClass(env, cursor);
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");

    if (moveToFirst == NULL || getString == NULL) {
        LogE("Failed to get cursor methods");
        (*env)->DeleteLocalRef(env, cursorClass);
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        jstring value = (*env)->CallObjectMethod(env, cursor, getString, 0);
        if (value != NULL) {
            const char* valueStr = (*env)->GetStringUTFChars(env, value, NULL);
            if (valueStr != NULL) {
                result = SafeStrdup(valueStr, "GetFileInfo");
                (*env)->ReleaseStringUTFChars(env, value, valueStr);
                LogD("Got value: %s", result);
            }
            (*env)->DeleteLocalRef(env, value);
        } else {
            LogD("Value is NULL for column: %s", column);
        }
    } else {
        LogD("Cursor is empty");
    }

    (*env)->DeleteLocalRef(env, cursorClass);
    (*env)->DeleteLocalRef(env, cursor);

    return result;
}

// Проверка, является ли URI директорией
static jboolean IsDirectoryUri(JNIEnv* env, jobject activity, const char* uriStr) {
    char* mimeType = GetFileInfo(env, activity, uriStr, "mime_type");
    if (mimeType == NULL) {
        LogE("Failed to get MIME type for: %s", uriStr);
        return JNI_FALSE;
    }

    jboolean isDirectory = (strcmp(mimeType, "vnd.android.document/directory") == 0);
    LogD("MIME type: %s, isDirectory: %d", mimeType, isDirectory);
    free(mimeType);
    return isDirectory;
}

// Получение имени файла
static char* GetFileName(JNIEnv* env, jobject activity, const char* uriStr) {
    char* name = GetFileInfo(env, activity, uriStr, "_display_name");
    if (name == NULL) {
        LogE("Failed to get name for: %s", uriStr);
    } else {
        LogD("Got name: %s for URI: %s", name, uriStr);
    }
    return name;
}

// Получение дочерних элементов для *конкретного* documentId в tree URI
static jobjectArray ListChildrenUrisForDocumentId(JNIEnv* env, jobject activity, const char* treeUriStr, const char* specificDocumentId) {
    LogD("ListChildrenUrisForDocumentId for tree URI: %s, documentId: %s", treeUriStr, specificDocumentId);

    // 1. Получаем ContentResolver
    jobject contentResolver = GetContentResolver(env, activity);
    if (contentResolver == NULL) {
        LogE("Failed to get ContentResolver in ListChildrenUrisForDocumentId");
        return NULL;
    }

    // 2. Парсим tree URI
    jobject treeUri = ParseUri(env, treeUriStr);
    if (treeUri == NULL) {
        LogE("Failed to parse tree URI in ListChildrenUrisForDocumentId: %s", treeUriStr);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    // 3. Получаем DocumentsContract класс
    jclass documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogE("Failed to find DocumentsContract class in ListChildrenUrisForDocumentId");
        (*env)->DeleteLocalRef(env, treeUri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    // 4. Строим URI для дочерних документов, используя tree URI и *конкретный* documentId
    jmethodID buildChildDocumentsUriMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
    if (buildChildDocumentsUriMethod == NULL) {
        LogE("Failed to get buildChildDocumentsUriUsingTree method in ListChildrenUrisForDocumentId");
        (*env)->DeleteLocalRef(env, documentsContractClass);
        (*env)->DeleteLocalRef(env, treeUri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jstring jspecificDocumentId = (*env)->NewStringUTF(env, specificDocumentId);
    if (jspecificDocumentId == NULL) {
         LogE("Failed to create jstring for specificDocumentId: %s", specificDocumentId);
         (*env)->DeleteLocalRef(env, documentsContractClass);
         (*env)->DeleteLocalRef(env, treeUri);
         (*env)->DeleteLocalRef(env, contentResolver);
         return NULL;
    }

    jobject childrenUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
        buildChildDocumentsUriMethod, treeUri, jspecificDocumentId);

    (*env)->DeleteLocalRef(env, jspecificDocumentId);
    (*env)->DeleteLocalRef(env, documentsContractClass);
    (*env)->DeleteLocalRef(env, treeUri);

    if (childrenUri == NULL) {
        LogE("buildChildDocumentsUriUsingTree returned NULL in ListChildrenUrisForDocumentId");
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    // 5. Выполняем запрос
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");

    if (queryMethod == NULL) {
        LogE("Failed to get query method in ListChildrenUrisForDocumentId");
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, childrenUri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
         LogE("Failed to find String class in ListChildrenUrisForDocumentId");
         (*env)->DeleteLocalRef(env, resolverClass);
         (*env)->DeleteLocalRef(env, childrenUri);
         (*env)->DeleteLocalRef(env, contentResolver);
         return NULL;
    }
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    if (projection == NULL) {
         LogE("Failed to create projection array in ListChildrenUrisForDocumentId");
         (*env)->DeleteLocalRef(env, stringClass);
         (*env)->DeleteLocalRef(env, resolverClass);
         (*env)->DeleteLocalRef(env, childrenUri);
         (*env)->DeleteLocalRef(env, contentResolver);
         return NULL;
    }
    jstring docIdCol = (*env)->NewStringUTF(env, "document_id");
    if (docIdCol == NULL) {
         LogE("Failed to create jstring for 'document_id' column in ListChildrenUrisForDocumentId");
         (*env)->DeleteLocalRef(env, projection);
         (*env)->DeleteLocalRef(env, stringClass);
         (*env)->DeleteLocalRef(env, resolverClass);
         (*env)->DeleteLocalRef(env, childrenUri);
         (*env)->DeleteLocalRef(env, contentResolver);
         return NULL;
    }
    (*env)->SetObjectArrayElement(env, projection, 0, docIdCol);

    // Проверим исключение после SetObjectArrayElement
    if (CheckException(env, "SetObjectArrayElement in ListChildrenUrisForDocumentId setup")) {
         LogE("Exception occurred during projection setup in ListChildrenUrisForDocumentId");
         (*env)->DeleteLocalRef(env, docIdCol);
         (*env)->DeleteLocalRef(env, projection);
         (*env)->DeleteLocalRef(env, stringClass);
         (*env)->DeleteLocalRef(env, resolverClass);
         (*env)->DeleteLocalRef(env, childrenUri);
         (*env)->DeleteLocalRef(env, contentResolver);
         return NULL;
    }

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        childrenUri, projection, NULL, NULL, NULL);

    // Очистка временных ссылок, использованных для query
    (*env)->DeleteLocalRef(env, docIdCol);
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, stringClass);
    (*env)->DeleteLocalRef(env, resolverClass);
    (*env)->DeleteLocalRef(env, childrenUri);
    (*env)->DeleteLocalRef(env, contentResolver);

    if (cursor == NULL) {
        LogE("Cursor is NULL for children query in ListChildrenUrisForDocumentId");
        return NULL;
    }

    // 6. Обрабатываем результаты
    jclass cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
         LogE("Failed to get Cursor class in ListChildrenUrisForDocumentId");
         (*env)->DeleteLocalRef(env, cursor);
         return NULL;
    }
    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID isAfterLast = (*env)->GetMethodID(env, cursorClass, "isAfterLast", "()Z");
    jmethodID moveToNext = (*env)->GetMethodID(env, cursorClass, "moveToNext", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");
    jmethodID getCount = (*env)->GetMethodID(env, cursorClass, "getCount", "()I");

    if (moveToFirst == NULL || isAfterLast == NULL || moveToNext == NULL || getString == NULL || getCount == NULL) {
        LogE("Failed to get cursor methods in ListChildrenUrisForDocumentId");
        (*env)->DeleteLocalRef(env, cursorClass);
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    jint count = (*env)->CallIntMethod(env, cursor, getCount);
    LogD("Found %d children in cursor in ListChildrenUrisForDocumentId", count);

    // --- НАЧАЛО ДОБАВЛЕННОГО ЛОГИРОВАНИЯ ---
    LogD("ListChildrenUrisForDocumentId: After getting count, about to create result array. count = %d", (int)count);
    jobjectArray result = (*env)->NewObjectArray(env, count, stringClass, NULL);
    LogD("ListChildrenUrisForDocumentId: After NewObjectArray. result = %p", result);
    if (result == NULL) {
        LogE("Failed to create result array in ListChildrenUrisForDocumentId");
        (*env)->DeleteLocalRef(env, cursorClass);
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    LogD("ListChildrenUrisForDocumentId: About to call cursor.moveToFirst");
    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        LogD("ListChildrenUrisForDocumentId: moveToFirst returned true, entering while loop");
        jint index = 0;
        LogD("ListChildrenUrisForDocumentId: Initial index = %d", (int)index);
        while (!(*env)->CallBooleanMethod(env, cursor, isAfterLast) && index < count) {
            LogD("ListChildrenUrisForDocumentId: Inside while loop, index = %d", (int)index);
            LogD("ListChildrenUrisForDocumentId: About to call cursor.getString(0)");
            jstring jchildDocumentId = (*env)->CallObjectMethod(env, cursor, getString, 0);
            LogD("ListChildrenUrisForDocumentId: After cursor.getString(0), jchildDocumentId = %p", jchildDocumentId);
            if (jchildDocumentId != NULL) {
                LogD("ListChildrenUrisForDocumentId: jchildDocumentId is not NULL, about to call GetStringUTFChars");
                const char* childDocIdStr = (*env)->GetStringUTFChars(env, jchildDocumentId, NULL);
                LogD("ListChildrenUrisForDocumentId: After GetStringUTFChars, childDocIdStr = %p", childDocIdStr);
                if (childDocIdStr != NULL) {
                    LogD("ListChildrenUrisForDocumentId: childDocIdStr is not NULL, about to call BuildDocumentUri");
                    LogD("ListChildrenUrisForDocumentId: g_original_tree_uri = %s", g_original_tree_uri);
                    LogD("ListChildrenUrisForDocumentId: childDocIdStr = %s", childDocIdStr);
                    char* childUri = BuildDocumentUri(env, activity, g_original_tree_uri, childDocIdStr);
                    LogD("ListChildrenUrisForDocumentId: After BuildDocumentUri, childUri = %p", childUri);
                    if (childUri != NULL) {
                        LogD("ListChildrenUrisForDocumentId: childUri is not NULL, about to call NewStringUTF");
                        jstring jchildUri = (*env)->NewStringUTF(env, childUri);
                        LogD("ListChildrenUrisForDocumentId: After NewStringUTF, jchildUri = %p", jchildUri);
                        if (jchildUri != NULL) {
                            LogD("ListChildrenUrisForDocumentId: jchildUri is not NULL, about to call SetObjectArrayElement");
                            LogD("ListChildrenUrisForDocumentId: result = %p, index = %d, jchildUri = %p", result, (int)index, jchildUri);
                            (*env)->SetObjectArrayElement(env, result, index, jchildUri);
                            // Проверим, не возникло ли исключения после SetObjectArrayElement
                            LogD("ListChildrenUrisForDocumentId: After SetObjectArrayElement, about to check for exception");
                            if (CheckException(env, "SetObjectArrayElement in ListChildrenUrisForDocumentId loop")) {
                                LogE("Exception occurred in SetObjectArrayElement, index: %d, childUri: %s", (int)index, childUri);
                                // Освобождаем ресурсы и выходим из цикла
                                LogD("ListChildrenUrisForDocumentId: About to ReleaseStringUTFChars");
                                (*env)->ReleaseStringUTFChars(env, jchildDocumentId, childDocIdStr);
                                LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef jchildDocumentId");
                                (*env)->DeleteLocalRef(env, jchildDocumentId);
                                LogD("ListChildrenUrisForDocumentId: About to free childUri");
                                free(childUri);
                                LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef jchildUri");
                                (*env)->DeleteLocalRef(env, jchildUri); // Освобождаем jchildUri, который не был добавлен
                                LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef cursorClass");
                                (*env)->DeleteLocalRef(env, cursorClass);
                                LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef cursor");
                                (*env)->DeleteLocalRef(env, cursor);
                                LogD("ListChildrenUrisForDocumentId: About to return NULL");
                                return NULL; // Прерываем выполнение функции
                            }
                            LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef jchildUri (after successful SetObjectArrayElement)");
                            (*env)->DeleteLocalRef(env, jchildUri);
                            LogD("ListChildrenUrisForDocumentId: About to free childUri (after successful SetObjectArrayElement)");
                            free(childUri);
                            LogD("ListChildrenUrisForDocumentId: About to increment index");
                            index++;
                            LogD("ListChildrenUrisForDocumentId: Index incremented, new value = %d", (int)index);
                            LogD("Added child URI in ListChildrenUrisForDocumentId: %s", childUri);
                        } else {
                             LogE("Failed to create jstring for child URI: %s", childUri);
                             LogD("ListChildrenUrisForDocumentId: About to free childUri (NewStringUTF failed)");
                             free(childUri);
                        }
                    } else {
                        LogE("Failed to build URI for child document ID: %s", childDocIdStr);
                    }
                    LogD("ListChildrenUrisForDocumentId: About to ReleaseStringUTFChars");
                    (*env)->ReleaseStringUTFChars(env, jchildDocumentId, childDocIdStr);
                } else {
                    LogE("GetStringUTFChars returned NULL for child document ID at index %d", (int)index);
                }
                LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef jchildDocumentId");
                (*env)->DeleteLocalRef(env, jchildDocumentId);
            } else {
                LogD("Child document ID is NULL at index %d", (int)index);
                // Возможно, строка в Cursor пустая или произошла ошибка, но не исключение.
                // Лучше прерваться, чтобы избежать бесконечного цикла.
                LogD("ListChildrenUrisForDocumentId: Breaking loop because jchildDocumentId is NULL");
                break;
            }
            // Переходим к следующему элементу, только если не было критической ошибки выше
            // CheckException внутри moveToNext не нужен, если он сам вызывает исключение,
            // оно будет поймано в CheckException на следующей итерации или снаружу.
            LogD("ListChildrenUrisForDocumentId: About to call cursor.moveToNext");
            (*env)->CallBooleanMethod(env, cursor, moveToNext);
        }
        LogD("ListChildrenUrisForDocumentId: Exited while loop");
    } else {
         LogD("Cursor is empty or moveToFirst failed in ListChildrenUrisForDocumentId");
    }

    LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef cursorClass");
    (*env)->DeleteLocalRef(env, cursorClass);
    LogD("ListChildrenUrisForDocumentId: About to DeleteLocalRef cursor");
    (*env)->DeleteLocalRef(env, cursor);

    LogD("ListChildrenUrisForDocumentId completed, returning %d URIs", count);
    return result;
}

// Рекурсивный обход директории
static void WalkDirectory(JNIEnv* env, jobject activity, const char* currentURI,
                         const char* dstDir, const char* relPath,
                         FileCopyTask** tasks, int* taskCount, int* maxTasks) {

    if (CheckException(env, "WalkDirectory entry")) {
        LogE("Exception at WalkDirectory entry");
        return;
    }

    LogD("WalkDirectory processing: %s with relPath: '%s'", currentURI, relPath);

    // Проверяем, является ли это первым вызовом для корня дерева
    jboolean isInitialRootCall = JNI_FALSE;
    char* initialRootDocumentId = NULL; // Для хранения documentId корня при initial call
    if (strlen(relPath) == 0 && strstr(currentURI, "/tree/") != NULL) {
        isInitialRootCall = JNI_TRUE;
        LogD("Identified as initial root call for tree URI.");
        // Получаем documentId корня
        initialRootDocumentId = GetTreeDocumentId(env, activity, currentURI);
        if (initialRootDocumentId == NULL) {
             LogE("Failed to get root document ID for initial root call.");
             return;
        }
        LogD("Initial root document ID: %s", initialRootDocumentId);
    }

    // Для tree URI (кроме первого вызова) сначала получаем document URI
    char* documentUri = NULL;
    if (strstr(currentURI, "/tree/") != NULL && !isInitialRootCall) {
        LogD("Converting tree URI to document URI (not initial call)");
        documentUri = GetRootDocumentUri(env, activity, currentURI);
        if (documentUri == NULL) {
            LogE("Failed to convert tree URI to document URI: %s", currentURI);
            free(initialRootDocumentId); // Освобождаем, если был initial call
            return;
        }
        LogD("Converted to document URI: %s", documentUri);
    } else {
        // Это уже document URI или это initial root call
        if (isInitialRootCall) {
             // Для initial root call documentUri получаем из tree URI и initialRootDocumentId
             documentUri = BuildDocumentUri(env, activity, currentURI, initialRootDocumentId);
             if (documentUri == NULL) {
                 LogE("Failed to build document URI for initial root: %s", currentURI);
                 free(initialRootDocumentId);
                 return;
             }
             LogD("Initial root document URI: %s", documentUri);
        } else {
             // Это document URI
             documentUri = SafeStrdup(currentURI, "WalkDirectory-documentUri");
        }
    }

    // Получаем имя файла/директории
    char* name = NULL;
    if (isInitialRootCall) {
        // Для initial root call мы получили initialRootDocumentId.
        // Извлекаем имя из initialRootDocumentId вручную.
        if (initialRootDocumentId != NULL) {
            const char* lastSlash = strrchr(initialRootDocumentId, ':');
            if (lastSlash != NULL) {
                const char* pathPart = lastSlash + 1;
                const char* finalName = strrchr(pathPart, '/');
                if (finalName != NULL) {
                    name = SafeStrdup(finalName + 1, "initial-root-name");
                } else {
                    name = SafeStrdup(pathPart, "initial-root-name");
                }
            } else {
                name = SafeStrdup(initialRootDocumentId, "initial-root-name");
            }
            LogD("Extracted name for initial root from documentId: %s", name);
        }
    } else {
        // Это не initial root call, получаем имя обычным способом
        name = GetFileName(env, activity, documentUri);
    }
    if (name == NULL) {
        LogE("Failed to get name for document URI: %s", documentUri);
        free(documentUri);
        free(initialRootDocumentId); // Освобождаем, если был initial call
        return;
    }

    // Строим относительный путь
    char* newRelPath = NULL;
    if (isInitialRootCall) {
        // Для корня дерева на первом уровне мы не включаем его имя в путь.
        // Вместо этого, мы сразу обрабатываем его дочерние элементы с пустым relPath.
        LogD("Processing initial root directory: %s, skipping name in relPath", name);
        free(name);
        free(documentUri);
        // Рекурсивно обходим дочерние элементы корня с пустым relPath
        // Для этого нам нужен initialRootDocumentId, чтобы вызвать ListChildrenUrisForDocumentId
        // initialRootDocumentId уже доступен
        jobjectArray children = ListChildrenUrisForDocumentId(env, activity, currentURI, initialRootDocumentId); // currentURI is tree URI, initialRootDocumentId
        free(initialRootDocumentId); // Освобождаем после использования
        if (children != NULL) {
            jsize childCount = (*env)->GetArrayLength(env, children);
            LogD("Found %d children in initial root directory", (int)childCount);

            for (jsize i = 0; i < childCount; i++) {
                jstring jchildUri = (*env)->GetObjectArrayElement(env, children, i);
                if (jchildUri != NULL) {
                    const char* childUri = (*env)->GetStringUTFChars(env, jchildUri, NULL);
                    if (childUri != NULL) {
                        LogD("Processing initial root child %d: %s", (int)i, childUri);
                        // Передаём пустой relPath для дочернего элемента корня
                        WalkDirectory(env, activity, childUri, dstDir, "", tasks, taskCount, maxTasks);
                        (*env)->ReleaseStringUTFChars(env, jchildUri, childUri);
                    }
                    (*env)->DeleteLocalRef(env, jchildUri);
                }

                if (CheckException(env, "WalkDirectory initial root iteration")) {
                    LogE("Exception in initial root iteration");
                    break;
                }
            }
            (*env)->DeleteLocalRef(env, children);
        } else {
            LogE("Failed to list children for initial root directory");
        }
        return; // Возвращаемся после обработки дочерних элементов корня
    } else {
        // Это не initial root call, формируем relPath как обычно
        if (strlen(relPath) == 0) {
            newRelPath = SafeStrdup(name, "relPath-normal-root");
        } else {
            size_t len = strlen(relPath) + strlen(name) + 2;
            newRelPath = SafeMalloc(len, "relPath-normal-child");
            if (newRelPath != NULL) {
                snprintf(newRelPath, len, "%s/%s", relPath, name);
            }
        }
    }


    if (newRelPath == NULL) {
        if (!isInitialRootCall) { // name не освобождается в isInitialRootCall
             free(name);
        }
        free(documentUri);
        free(initialRootDocumentId); // Освобождаем, если был initial call
        return;
    }

    LogD("Name: %s, Relative path: %s", name, newRelPath);

    // Проверяем тип
    jboolean isDirectory = IsDirectoryUri(env, activity, documentUri);
    if (CheckException(env, "IsDirectoryUri")) {
        LogE("Exception in IsDirectoryUri");
        if (!isInitialRootCall) {
             free(name);
        }
        free(newRelPath);
        free(documentUri);
        free(initialRootDocumentId); // Освобождаем, если был initial call
        return;
    }

    if (isDirectory) {
        LogD("It's a directory: %s", name);

        // Получаем documentId текущей директории (documentUri)
        char* currentDirDocumentId = GetDocumentId(env, ParseUri(env, documentUri));
        if (currentDirDocumentId == NULL) {
            LogE("Failed to get document ID for directory: %s", documentUri);
            if (!isInitialRootCall) {
                 free(name);
            }
            free(newRelPath);
            free(documentUri);
            free(initialRootDocumentId); // Освобождаем, если был initial call
            return;
        }

        // Вызываем исправленную функцию с глобальной переменной
        // g_original_tree_uri должна быть установлена в GetAllFilesForCopy
        if (g_original_tree_uri == NULL) {
             LogE("g_original_tree_uri is NULL in WalkDirectory. This should not happen if called correctly from GetAllFilesForCopy.");
             free(currentDirDocumentId);
             if (!isInitialRootCall) {
                 free(name);
             }
             free(newRelPath);
             free(documentUri);
             free(initialRootDocumentId); // Освобождаем, если был initial call
             return;
        }
        jobjectArray children = ListChildrenUrisForDocumentId(env, activity, g_original_tree_uri, currentDirDocumentId);
        free(currentDirDocumentId); // Освобождаем полученный documentId

        if (children != NULL) {
            jsize childCount = (*env)->GetArrayLength(env, children);
            LogD("Found %d children in directory %s", (int)childCount, name);

            for (jsize i = 0; i < childCount; i++) {
                jstring jchildUri = (*env)->GetObjectArrayElement(env, children, i);
                if (jchildUri != NULL) {
                    const char* childUri = (*env)->GetStringUTFChars(env, jchildUri, NULL);
                    if (childUri != NULL) {
                        LogD("Processing child %d: %s", (int)i, childUri);
                        // Передаём *новый* relPath (имя текущей директории) в рекурсивный вызов
                        WalkDirectory(env, activity, childUri, dstDir, newRelPath, tasks, taskCount, maxTasks);
                        (*env)->ReleaseStringUTFChars(env, jchildUri, childUri);
                    }
                    (*env)->DeleteLocalRef(env, jchildUri);
                }

                if (CheckException(env, "WalkDirectory iteration")) {
                    LogE("Exception in iteration");
                    break;
                }
            }
            (*env)->DeleteLocalRef(env, children);
        } else {
            LogE("Failed to list children for directory: %s", name);
        }
    } else {
        LogD("It's a file: %s", name);

        // Добавляем файл в список задач
        if (*taskCount >= *maxTasks) {
            int newMaxTasks = *maxTasks * 2;
            FileCopyTask* newTasks = realloc(*tasks, newMaxTasks * sizeof(FileCopyTask));
            if (newTasks != NULL) {
                *tasks = newTasks;
                *maxTasks = newMaxTasks;
                LogD("Increased tasks array to %d", newMaxTasks);
            }
        }

        if (*taskCount < *maxTasks) {
            FileCopyTask* task = &(*tasks)[*taskCount];
            task->srcURI = SafeStrdup(documentUri, "task-srcURI");

            size_t dstPathLen = strlen(dstDir) + strlen(newRelPath) + 2;
            task->dstPath = SafeMalloc(dstPathLen, "task-dstPath");

            if (task->srcURI != NULL && task->dstPath != NULL) {
                snprintf(task->dstPath, dstPathLen, "%s/%s", dstDir, newRelPath);
                (*taskCount)++;
                LogD("Added copy task: %s -> %s", task->srcURI, task->dstPath);
            } else {
                LogE("Failed to allocate memory for copy task");
                if (task->srcURI != NULL) free(task->srcURI);
                if (task->dstPath != NULL) free(task->dstPath);
            }
        }
    }

    if (!isInitialRootCall) { // name не освобождается в isInitialRootCall
         free(name);
    }
    free(newRelPath);
    free(documentUri);
    free(initialRootDocumentId); // Освобождаем, если был initial call (даже если не был, free(NULL) безопасен)
}

// Основная функция
static FileCopyTask* GetAllFilesForCopy(JNIEnv* env, jobject activity,
                                       const char* srcURI, const char* dstDir,
                                       int* fileCount) {
    LogD("GetAllFilesForCopy started: %s -> %s", srcURI, dstDir);

    // Устанавливаем глобальную переменную
    g_original_tree_uri = srcURI;

    int maxTasks = 100;
    int taskCount = 0;
    FileCopyTask* tasks = SafeMalloc(maxTasks * sizeof(FileCopyTask), "tasks-init");
    if (tasks == NULL) {
        *fileCount = 0;
        g_original_tree_uri = NULL; // Сбрасываем
        return NULL;
    }

    WalkDirectory(env, activity, srcURI, dstDir, "", &tasks, &taskCount, &maxTasks);

    if (CheckException(env, "GetAllFilesForCopy after walk")) {
        LogE("Exception during directory walk");
    }

    *fileCount = taskCount;
    g_original_tree_uri = NULL; // Сбрасываем
    LogD("GetAllFilesForCopy completed: found %d files", taskCount);
    return tasks;
}

// Освобождение памяти
static void FreeFileCopyTasks(FileCopyTask* tasks, int count) {
    if (tasks == NULL) return;

    for (int i = 0; i < count; i++) {
        if (tasks[i].srcURI != NULL) free(tasks[i].srcURI);
        if (tasks[i].dstPath != NULL) free(tasks[i].dstPath);
    }
    free(tasks);
}
*/
import "C"
import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

// FileCopyTask представляет задачу копирования файла
type FileCopyTask struct {
	SrcURI  string
	DstPath string
}

// GetAllFilesForCopyJNI получает все файлы для копирования через JNI
func GetAllFilesForCopyJNI(srcURI, dstDir string) ([]FileCopyTask, error) {
	var tasks []FileCopyTask

	err := driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		cSrcURI := C.CString(srcURI)
		cDstDir := C.CString(dstDir)
		defer func() {
			C.free(unsafe.Pointer(cSrcURI))
			C.free(unsafe.Pointer(cDstDir))
		}()

		var fileCount C.int
		cTasks := C.GetAllFilesForCopy(env, activity, cSrcURI, cDstDir, &fileCount)
		if cTasks == nil {
			return errors.New("failed to get file list from JNI")
		}
		defer C.FreeFileCopyTasks(cTasks, fileCount)

		if fileCount > 0 {
			taskSlice := (*[1 << 30]C.FileCopyTask)(unsafe.Pointer(cTasks))[:fileCount:fileCount]
			for i := 0; i < int(fileCount); i++ {
				task := taskSlice[i]
				tasks = append(tasks, FileCopyTask{
					SrcURI:  C.GoString(task.srcURI),
					DstPath: C.GoString(task.dstPath),
				})
			}
		}

		return nil
	})

	return tasks, err
}

// CopyDirectoryJNI копирует директорию через JNI
func CopyDirectoryJNI(srcURI, dstDir string, copyFileFn func(srcURI, dstPath string) error) error {
	log.Trace(srcURI, dstDir)
	// Создаём целевую директорию перед копированием
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		return err
	}

	// Передаём dstDir как базовую директорию для копирования
	// C-код (после исправления) будет формировать пути относительно dstDir,
	// начиная с содержимого дерева, указанного в srcURI
	tasks, err := GetAllFilesForCopyJNI(srcURI, dstDir)
	if err != nil {
		return err
	}
	log.Tracef("tasks %v", tasks)

	// Создаем необходимые директории
	dirs := make(map[string]bool)
	for _, task := range tasks {
		dir := filepath.Dir(task.DstPath)
		dirs[dir] = true
	}
	for dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}

	// Копируем файлы
	for _, task := range tasks {
		if err := copyFileFn(task.SrcURI, task.DstPath); err != nil {
			return err
		}
	}

	return nil
}
