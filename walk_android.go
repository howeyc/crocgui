//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <android/log.h>

#define LOG_TAG "FileCopy"
#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, LOG_TAG, __VA_ARGS__)
#define LogE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)

// Структура для хранения путей файлов
typedef struct {
    char* srcURI;
    char* dstPath;
} FileCopyTask;

// Глобальные переменные для кэширования
static jclass uriClass = NULL;
static jclass documentsContractClass = NULL;
static jclass stringClass = NULL;
static jclass activityClass = NULL;

static jmethodID parseMethod = NULL;
static jmethodID getContentResolverMethod = NULL;
static jmethodID buildChildDocumentsUriMethod = NULL;
static jmethodID buildDocumentUriMethod = NULL;
static jmethodID queryMethod = NULL;
static jmethodID moveToFirstMethod = NULL;
static jmethodID isAfterLastMethod = NULL;
static jmethodID moveToNextMethod = NULL;
static jmethodID getStringMethod = NULL;
static jmethodID getCountMethod = NULL;
static jmethodID toStringMethod = NULL;

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

// Безопасное выделение памяти с проверкой OOM
static void* SafeMalloc(size_t size, const char* context) {
    void* ptr = malloc(size);
    if (ptr == NULL) {
        LogE("OutOfMemoryError in %s: failed to allocate %zu bytes", context, size);
    }
    return ptr;
}

static void* SafeRealloc(void* ptr, size_t size, const char* context) {
    void* new_ptr = realloc(ptr, size);
    if (new_ptr == NULL) {
        LogE("OutOfMemoryError in %s: failed to reallocate %zu bytes", context, size);
        free(ptr);
    }
    return new_ptr;
}

static char* SafeStrdup(const char* str, const char* context) {
    if (str == NULL) {
        LogE("SafeStrdup: NULL string in %s", context);
        return NULL;
    }
    char* new_str = strdup(str);
    if (new_str == NULL) {
        LogE("OutOfMemoryError in %s: failed to duplicate string", context);
    }
    return new_str;
}

// Инициализация классов и методов
static jboolean InitializeJNIClasses(JNIEnv* env) {
    if (uriClass == NULL) {
        jclass localUriClass = (*env)->FindClass(env, "android/net/Uri");
        if (localUriClass == NULL) {
            CheckException(env, "FindClass Uri");
            return JNI_FALSE;
        }
        uriClass = (*env)->NewGlobalRef(env, localUriClass);
        (*env)->DeleteLocalRef(env, localUriClass);

        parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
        if (parseMethod == NULL) {
            CheckException(env, "GetStaticMethodID parse");
            return JNI_FALSE;
        }

        toStringMethod = (*env)->GetMethodID(env, uriClass, "toString", "()Ljava/lang/String;");
        if (toStringMethod == NULL) {
            CheckException(env, "GetMethodID toString");
            return JNI_FALSE;
        }
    }

    if (documentsContractClass == NULL) {
        jclass localDocsClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
        if (localDocsClass == NULL) {
            CheckException(env, "FindClass DocumentsContract");
            return JNI_FALSE;
        }
        documentsContractClass = (*env)->NewGlobalRef(env, localDocsClass);
        (*env)->DeleteLocalRef(env, localDocsClass);

        buildChildDocumentsUriMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
            "buildChildDocumentsUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
        if (buildChildDocumentsUriMethod == NULL) {
            CheckException(env, "GetStaticMethodID buildChildDocumentsUriUsingTree");
            return JNI_FALSE;
        }

        buildDocumentUriMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
            "buildDocumentUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");
        if (buildDocumentUriMethod == NULL) {
            CheckException(env, "GetStaticMethodID buildDocumentUriUsingTree");
            return JNI_FALSE;
        }
    }

    if (stringClass == NULL) {
        jclass localStringClass = (*env)->FindClass(env, "java/lang/String");
        if (localStringClass == NULL) {
            CheckException(env, "FindClass String");
            return JNI_FALSE;
        }
        stringClass = (*env)->NewGlobalRef(env, localStringClass);
        (*env)->DeleteLocalRef(env, localStringClass);
    }

    if (activityClass == NULL) {
        jclass localActivityClass = (*env)->FindClass(env, "android/app/Activity");
        if (localActivityClass == NULL) {
            CheckException(env, "FindClass Activity");
            return JNI_FALSE;
        }
        activityClass = (*env)->NewGlobalRef(env, localActivityClass);
        (*env)->DeleteLocalRef(env, localActivityClass);

        getContentResolverMethod = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
        if (getContentResolverMethod == NULL) {
            CheckException(env, "GetMethodID getContentResolver");
            return JNI_FALSE;
        }
    }

    return JNI_TRUE;
}

// Получение ContentResolver
static jobject GetContentResolver(JNIEnv* env, jobject activity) {
    jobject resolver = (*env)->CallObjectMethod(env, activity, getContentResolverMethod);
    if (CheckException(env, "GetContentResolver") || resolver == NULL) {
        LogE("Failed to get ContentResolver");
        return NULL;
    }
    return resolver;
}

// Парсинг URI
static jobject ParseUri(JNIEnv* env, const char* uriStr) {
    jstring juriStr = (*env)->NewStringUTF(env, uriStr);
    if (juriStr == NULL) {
        CheckException(env, "NewStringUTF for URI");
        return NULL;
    }

    jobject uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    (*env)->DeleteLocalRef(env, juriStr);

    if (CheckException(env, "ParseUri") || uri == NULL) {
        LogE("Failed to parse URI: %s", uriStr);
        return NULL;
    }

    return uri;
}

// IsDirectoryUri - проверка, является ли URI директорией
static jboolean IsDirectoryUri(JNIEnv* env, jobject activity, const char* uriStr) {
    if (!InitializeJNIClasses(env)) {
        LogE("Failed to initialize classes in IsDirectoryUri");
        return JNI_FALSE;
    }

    jobject uri = ParseUri(env, uriStr);
    if (uri == NULL) {
        return JNI_FALSE;
    }

    jobject contentResolver = GetContentResolver(env, activity);
    if (contentResolver == NULL) {
        (*env)->DeleteLocalRef(env, uri);
        return JNI_FALSE;
    }

    // Получаем класс ContentResolver
    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        CheckException(env, "GetObjectClass for ContentResolver");
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    // Получаем метод query
    if (queryMethod == NULL) {
        queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
            "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
        if (queryMethod == NULL) {
            CheckException(env, "GetMethodID query");
            (*env)->DeleteLocalRef(env, resolverClass);
            (*env)->DeleteLocalRef(env, uri);
            (*env)->DeleteLocalRef(env, contentResolver);
            return JNI_FALSE;
        }
    }

    // Создаем проекцию для MIME типа
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    if (projection == NULL) {
        CheckException(env, "NewObjectArray for projection");
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    jstring mimeTypeCol = (*env)->NewStringUTF(env, "mime_type");
    if (mimeTypeCol == NULL) {
        CheckException(env, "NewStringUTF for mime_type");
        (*env)->DeleteLocalRef(env, projection);
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return JNI_FALSE;
    }

    (*env)->SetObjectArrayElement(env, projection, 0, mimeTypeCol);
    (*env)->DeleteLocalRef(env, mimeTypeCol);

    // Выполняем запрос
    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Очистка
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, resolverClass);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, contentResolver);

    if (CheckException(env, "Query in IsDirectoryUri") || cursor == NULL) {
        LogE("Failed to query URI in IsDirectoryUri: %s", uriStr);
        return JNI_FALSE;
    }

    // Получаем класс Cursor
    jclass localCursorClass = (*env)->GetObjectClass(env, cursor);
    if (localCursorClass == NULL) {
        CheckException(env, "GetObjectClass for Cursor");
        (*env)->DeleteLocalRef(env, cursor);
        return JNI_FALSE;
    }

    // Получаем методы Cursor
    if (moveToFirstMethod == NULL) {
        moveToFirstMethod = (*env)->GetMethodID(env, localCursorClass, "moveToFirst", "()Z");
        if (moveToFirstMethod == NULL) {
            CheckException(env, "GetMethodID moveToFirst");
            (*env)->DeleteLocalRef(env, localCursorClass);
            (*env)->DeleteLocalRef(env, cursor);
            return JNI_FALSE;
        }
    }

    if (getStringMethod == NULL) {
        getStringMethod = (*env)->GetMethodID(env, localCursorClass, "getString", "(I)Ljava/lang/String;");
        if (getStringMethod == NULL) {
            CheckException(env, "GetMethodID getString");
            (*env)->DeleteLocalRef(env, localCursorClass);
            (*env)->DeleteLocalRef(env, cursor);
            return JNI_FALSE;
        }
    }

    jboolean isDirectory = JNI_FALSE;

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirstMethod)) {
        jstring mimeType = (*env)->CallObjectMethod(env, cursor, getStringMethod, 0);
        if (mimeType != NULL) {
            const char* mimeTypeStr = (*env)->GetStringUTFChars(env, mimeType, NULL);
            if (mimeTypeStr != NULL) {
                if (strcmp(mimeTypeStr, "vnd.android.document/directory") == 0) {
                    isDirectory = JNI_TRUE;
                }
                (*env)->ReleaseStringUTFChars(env, mimeType, mimeTypeStr);
            }
            (*env)->DeleteLocalRef(env, mimeType);
        }
    }

    (*env)->DeleteLocalRef(env, localCursorClass);
    (*env)->DeleteLocalRef(env, cursor);

    return isDirectory;
}

// GetFileName - получение имени файла из URI
static char* GetFileName(JNIEnv* env, jobject activity, const char* uriStr) {
    if (!InitializeJNIClasses(env)) {
        LogE("Failed to initialize classes in GetFileName");
        return NULL;
    }

    jobject uri = ParseUri(env, uriStr);
    if (uri == NULL) {
        return NULL;
    }

    jobject contentResolver = GetContentResolver(env, activity);
    if (contentResolver == NULL) {
        (*env)->DeleteLocalRef(env, uri);
        return NULL;
    }

    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        CheckException(env, "GetObjectClass for ContentResolver in GetFileName");
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    if (queryMethod == NULL) {
        queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
            "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
        if (queryMethod == NULL) {
            CheckException(env, "GetMethodID query in GetFileName");
            (*env)->DeleteLocalRef(env, resolverClass);
            (*env)->DeleteLocalRef(env, uri);
            (*env)->DeleteLocalRef(env, contentResolver);
            return NULL;
        }
    }

    // Проекция для имени файла
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    if (projection == NULL) {
        CheckException(env, "NewObjectArray for projection in GetFileName");
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jstring displayNameCol = (*env)->NewStringUTF(env, "_display_name");
    if (displayNameCol == NULL) {
        CheckException(env, "NewStringUTF for _display_name");
        (*env)->DeleteLocalRef(env, projection);
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, uri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    (*env)->SetObjectArrayElement(env, projection, 0, displayNameCol);
    (*env)->DeleteLocalRef(env, displayNameCol);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        uri, projection, NULL, NULL, NULL);

    // Очистка
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, resolverClass);
    (*env)->DeleteLocalRef(env, uri);
    (*env)->DeleteLocalRef(env, contentResolver);

    if (CheckException(env, "Query in GetFileName") || cursor == NULL) {
        LogE("Failed to query URI in GetFileName: %s", uriStr);
        return NULL;
    }

    jclass localCursorClass = (*env)->GetObjectClass(env, cursor);
    if (localCursorClass == NULL) {
        CheckException(env, "GetObjectClass for Cursor in GetFileName");
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    if (moveToFirstMethod == NULL) {
        moveToFirstMethod = (*env)->GetMethodID(env, localCursorClass, "moveToFirst", "()Z");
    }
    if (getStringMethod == NULL) {
        getStringMethod = (*env)->GetMethodID(env, localCursorClass, "getString", "(I)Ljava/lang/String;");
    }

    char* result = NULL;

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirstMethod)) {
        jstring name = (*env)->CallObjectMethod(env, cursor, getStringMethod, 0);
        if (name != NULL) {
            const char* utfStr = (*env)->GetStringUTFChars(env, name, NULL);
            if (utfStr != NULL) {
                result = SafeStrdup(utfStr, "GetFileName");
                (*env)->ReleaseStringUTFChars(env, name, utfStr);
            }
            (*env)->DeleteLocalRef(env, name);
        }
    }

    (*env)->DeleteLocalRef(env, localCursorClass);
    (*env)->DeleteLocalRef(env, cursor);

    if (result == NULL) {
        LogE("Failed to get file name for URI: %s", uriStr);
    }

    return result;
}

// ListChildrenUris - получение списка дочерних URI
static jobjectArray ListChildrenUris(JNIEnv* env, jobject activity, const char* uriStr) {
    if (!InitializeJNIClasses(env)) {
        LogE("Failed to initialize classes in ListChildrenUris");
        return NULL;
    }

    jobject treeUri = ParseUri(env, uriStr);
    if (treeUri == NULL) {
        return NULL;
    }

    // Строим URI для дочерних документов
    jobject childrenUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
        buildChildDocumentsUriMethod, treeUri, NULL);

    if (CheckException(env, "buildChildDocumentsUriUsingTree") || childrenUri == NULL) {
        LogE("Failed to build children URI for: %s", uriStr);
        (*env)->DeleteLocalRef(env, treeUri);
        return NULL;
    }

    jobject contentResolver = GetContentResolver(env, activity);
    if (contentResolver == NULL) {
        (*env)->DeleteLocalRef(env, treeUri);
        (*env)->DeleteLocalRef(env, childrenUri);
        return NULL;
    }

    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        CheckException(env, "GetObjectClass for ContentResolver in ListChildrenUris");
        (*env)->DeleteLocalRef(env, treeUri);
        (*env)->DeleteLocalRef(env, childrenUri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    if (queryMethod == NULL) {
        queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
            "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    }

    // Проекция для document_id
    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    if (projection == NULL) {
        CheckException(env, "NewObjectArray for projection in ListChildrenUris");
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, treeUri);
        (*env)->DeleteLocalRef(env, childrenUri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    jstring docIdCol = (*env)->NewStringUTF(env, "document_id");
    if (docIdCol == NULL) {
        CheckException(env, "NewStringUTF for document_id");
        (*env)->DeleteLocalRef(env, projection);
        (*env)->DeleteLocalRef(env, resolverClass);
        (*env)->DeleteLocalRef(env, treeUri);
        (*env)->DeleteLocalRef(env, childrenUri);
        (*env)->DeleteLocalRef(env, contentResolver);
        return NULL;
    }

    (*env)->SetObjectArrayElement(env, projection, 0, docIdCol);
    (*env)->DeleteLocalRef(env, docIdCol);

    jobject cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod,
        childrenUri, projection, NULL, NULL, NULL);

    // Очистка временных ссылок
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, resolverClass);
    (*env)->DeleteLocalRef(env, childrenUri);
    (*env)->DeleteLocalRef(env, treeUri);
    (*env)->DeleteLocalRef(env, contentResolver);

    if (CheckException(env, "Query in ListChildrenUris") || cursor == NULL) {
        LogE("Failed to query children for URI: %s", uriStr);
        return NULL;
    }

    jclass localCursorClass = (*env)->GetObjectClass(env, cursor);
    if (localCursorClass == NULL) {
        CheckException(env, "GetObjectClass for Cursor in ListChildrenUris");
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    if (moveToFirstMethod == NULL) moveToFirstMethod = (*env)->GetMethodID(env, localCursorClass, "moveToFirst", "()Z");
    if (isAfterLastMethod == NULL) isAfterLastMethod = (*env)->GetMethodID(env, localCursorClass, "isAfterLast", "()Z");
    if (moveToNextMethod == NULL) moveToNextMethod = (*env)->GetMethodID(env, localCursorClass, "moveToNext", "()Z");
    if (getStringMethod == NULL) getStringMethod = (*env)->GetMethodID(env, localCursorClass, "getString", "(I)Ljava/lang/String;");
    if (getCountMethod == NULL) getCountMethod = (*env)->GetMethodID(env, localCursorClass, "getCount", "()I");

    // Получаем количество элементов
    jint count = (*env)->CallIntMethod(env, cursor, getCountMethod);
    if (CheckException(env, "getCount in ListChildrenUris")) {
        (*env)->DeleteLocalRef(env, localCursorClass);
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    // Создаем массив результатов
    jobjectArray result = (*env)->NewObjectArray(env, count, stringClass, NULL);
    if (result == NULL) {
        CheckException(env, "NewObjectArray for result in ListChildrenUris");
        (*env)->DeleteLocalRef(env, localCursorClass);
        (*env)->DeleteLocalRef(env, cursor);
        return NULL;
    }

    if ((*env)->CallBooleanMethod(env, cursor, moveToFirstMethod)) {
        jint index = 0;
        while (!(*env)->CallBooleanMethod(env, cursor, isAfterLastMethod) && index < count) {
            jstring documentId = (*env)->CallObjectMethod(env, cursor, getStringMethod, 0);
            if (documentId != NULL) {
                // Строим полный URI документа
                jobject documentUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                    buildDocumentUriMethod, treeUri, documentId);

                if (documentUri != NULL) {
                    jstring uriString = (*env)->CallObjectMethod(env, documentUri, toStringMethod);
                    (*env)->SetObjectArrayElement(env, result, index, uriString);
                    (*env)->DeleteLocalRef(env, uriString);
                    (*env)->DeleteLocalRef(env, documentUri);
                }
                (*env)->DeleteLocalRef(env, documentId);
            }

            (*env)->CallBooleanMethod(env, cursor, moveToNextMethod);
            index++;
        }
    }

    (*env)->DeleteLocalRef(env, localCursorClass);
    (*env)->DeleteLocalRef(env, cursor);

    return result;
}

// Рекурсивная функция обхода директории
static void WalkDirectory(JNIEnv* env, jobject activity, const char* currentURI,
                         const char* dstDir, const char* relPath,
                         FileCopyTask** tasks, int* taskCount, int* maxTasks) {

    // Проверяем отмену через исключение
    if (CheckException(env, "WalkDirectory entry")) {
        return;
    }

    // Проверяем, является ли URI директорией
    jboolean isDirectory = IsDirectoryUri(env, activity, currentURI);
    if (CheckException(env, "IsDirectoryUri in WalkDirectory")) {
        return;
    }

    // Получаем имя файла/директории
    char* name = GetFileName(env, activity, currentURI);
    if (name == NULL) {
        LogE("Failed to get name for URI in WalkDirectory: %s", currentURI);
        return;
    }

    // Строим относительный путь
    char* newRelPath = NULL;
    if (strlen(relPath) == 0) {
        newRelPath = SafeStrdup(name, "WalkDirectory relPath1");
    } else {
        size_t newLen = strlen(relPath) + strlen(name) + 2;
        newRelPath = SafeMalloc(newLen, "WalkDirectory relPath2");
        if (newRelPath != NULL) {
            snprintf(newRelPath, newLen, "%s/%s", relPath, name);
        }
    }

    if (newRelPath == NULL) {
        free(name);
        return;
    }

    if (isDirectory) {
        // Рекурсивно обходим дочерние элементы
        jobjectArray children = ListChildrenUris(env, activity, currentURI);
        if (children != NULL) {
            jsize childCount = (*env)->GetArrayLength(env, children);
            LogD("Walking directory %s, found %d children", currentURI, (int)childCount);

            for (jsize i = 0; i < childCount; i++) {
                jstring jchildUri = (*env)->GetObjectArrayElement(env, children, i);
                if (jchildUri == NULL) {
                    continue;
                }

                const char* childUri = (*env)->GetStringUTFChars(env, jchildUri, NULL);
                if (childUri != NULL) {
                    WalkDirectory(env, activity, childUri, dstDir, newRelPath, tasks, taskCount, maxTasks);
                    (*env)->ReleaseStringUTFChars(env, jchildUri, childUri);
                }
                (*env)->DeleteLocalRef(env, jchildUri);

                // Проверяем исключения после каждой итерации
                if (CheckException(env, "WalkDirectory iteration")) {
                    break;
                }
            }
            (*env)->DeleteLocalRef(env, children);
        } else {
            LogE("Failed to list children for directory: %s", currentURI);
        }
    } else {
        // Добавляем файл в список задач
        if (*taskCount >= *maxTasks) {
            // Увеличиваем размер массива
            int newMaxTasks = *maxTasks * 2;
            FileCopyTask* newTasks = SafeRealloc(*tasks, newMaxTasks * sizeof(FileCopyTask),
                                               "WalkDirectory realloc");
            if (newTasks == NULL) {
                free(newRelPath);
                free(name);
                return;
            }
            *tasks = newTasks;
            *maxTasks = newMaxTasks;
            LogD("Increased tasks array to %d", newMaxTasks);
        }

        // Создаем задачу копирования
        FileCopyTask* task = &(*tasks)[*taskCount];
        task->srcURI = SafeStrdup(currentURI, "WalkDirectory srcURI");

        // Строим полный путь назначения
        size_t dstPathLen = strlen(dstDir) + strlen(newRelPath) + 2;
        char* dstPath = SafeMalloc(dstPathLen, "WalkDirectory dstPath");
        if (dstPath != NULL && task->srcURI != NULL) {
            snprintf(dstPath, dstPathLen, "%s/%s", dstDir, newRelPath);
            task->dstPath = dstPath;
            (*taskCount)++;
            LogD("Added copy task: %s -> %s", task->srcURI, task->dstPath);
        } else {
            // Освобождаем память в случае ошибки
            if (task->srcURI != NULL) free(task->srcURI);
            if (dstPath != NULL) free(dstPath);
            LogE("Failed to allocate memory for copy task");
        }
    }

    free(newRelPath);
    free(name);
}

// Основная функция для получения всех файлов для копирования
static FileCopyTask* GetAllFilesForCopy(JNIEnv* env, jobject activity,
                                       const char* srcURI, const char* dstDir,
                                       int* fileCount) {

    LogD("GetAllFilesForCopy started: %s -> %s", srcURI, dstDir);

    if (!InitializeJNIClasses(env)) {
        LogE("Failed to initialize JNI classes");
        *fileCount = 0;
        return NULL;
    }

    // Инициализируем массив задач
    int maxTasks = 100;
    int taskCount = 0;
    FileCopyTask* tasks = SafeMalloc(maxTasks * sizeof(FileCopyTask), "GetAllFilesForCopy initial");
    if (tasks == NULL) {
        *fileCount = 0;
        return NULL;
    }

    // Запускаем рекурсивный обход
    WalkDirectory(env, activity, srcURI, dstDir, "", &tasks, &taskCount, &maxTasks);

    if (CheckException(env, "GetAllFilesForCopy after WalkDirectory")) {
        LogE("Exception occurred during directory walk");
        // Продолжаем, возвращаем то, что успели собрать
    }

    *fileCount = taskCount;
    LogD("GetAllFilesForCopy completed: found %d files", taskCount);

    return tasks;
}

// Освобождение памяти задач
static void FreeFileCopyTasks(FileCopyTask* tasks, int count) {
    if (tasks == NULL) return;

    for (int i = 0; i < count; i++) {
        if (tasks[i].srcURI != NULL) {
            free(tasks[i].srcURI);
        }
        if (tasks[i].dstPath != NULL) {
            free(tasks[i].dstPath);
        }
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

		// Конвертируем C массив в Go slice
		if fileCount > 0 {
			taskSlice := (*[1 << 30]C.FileCopyTask)(unsafe.Pointer(cTasks))[:fileCount:fileCount]

			for i := 0; i < int(fileCount); i++ {
				task := taskSlice[i]
				goTask := FileCopyTask{
					SrcURI:  C.GoString(task.srcURI),
					DstPath: C.GoString(task.dstPath),
				}
				tasks = append(tasks, goTask)
			}
		}

		return nil
	})

	return tasks, err
}

// createAllDirectories создает все необходимые директории
func createAllDirectories(tasks []FileCopyTask) error {
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

	return nil
}

// CopyDirectoryJNI упрощенная версия без отмены
func CopyDirectoryJNI(srcURI, dstDir string, copyFileFn CopyFunc) error {
	if err := os.MkdirAll(dstDir, 0700); err != nil {
		return err
	}

	tasks, err := GetAllFilesForCopyJNI(srcURI, dstDir)
	if err != nil {
		return err
	}

	if err := createAllDirectories(tasks); err != nil {
		return err
	}

	for _, task := range tasks {
		if err := copyFileFn(task.SrcURI, task.DstPath); err != nil {
			return err
		}
	}

	return nil
}
