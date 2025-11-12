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

static char* getChildrenList0(JNIEnv* env, jobject activity, const char* uriStr) {
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

    char* result = NULL;
    char* currentBuffer = NULL;
    size_t bufferSize = 1024;
    size_t currentLength = 0;

    // Инициализация ContentResolver (оставляем как было)
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

    // Парсинг URI (оставляем как было)
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

    // Поиск DocumentsContract класса
    documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogD("getChildrenList: Failed to find DocumentsContract class");
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

    // Метод 1: buildChildDocumentsUriUsingTree (основной) - как в countChildren
    if (buildChildDocumentsUriUsingTreeMethod != NULL && getTreeDocumentIdMethod != NULL) {
        jstring treeDocId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getTreeDocumentIdMethod, uri);
        if (caseException(env, "getTreeDocumentId")) {
            LogD("getChildrenList: getTreeDocumentId failed with exception");
        } else if (treeDocId != NULL) {
            childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                buildChildDocumentsUriUsingTreeMethod, uri, treeDocId);
            if (caseException(env, "buildChildDocumentsUriUsingTree")) {
                if (childUri != NULL) {
                    (*env)->DeleteLocalRef(env, childUri);
                    childUri = NULL;
                }
            } else if (childUri != NULL) {
                childUriNeedsCleanup = JNI_TRUE;
                LogD("getChildrenList: Successfully built child URI using tree method");
            }
            (*env)->DeleteLocalRef(env, treeDocId);
        }
    }

    // Метод 2: buildChildDocumentsUri (альтернативный) - как в countChildren
    if (childUri == NULL && buildChildDocumentsUriMethod != NULL && getDocumentIdMethod != NULL) {
        jstring docId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getDocumentIdMethod, uri);
        if (caseException(env, "getDocumentId")) {
            LogD("getChildrenList: getDocumentId failed with exception");
        } else if (docId != NULL) {
            childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                buildChildDocumentsUriMethod, uri, docId);
            if (caseException(env, "buildChildDocumentsUri")) {
                if (childUri != NULL) {
                    (*env)->DeleteLocalRef(env, childUri);
                    childUri = NULL;
                }
            } else if (childUri != NULL) {
                childUriNeedsCleanup = JNI_TRUE;
                LogD("getChildrenList: Successfully built child URI using document method");
            }
            (*env)->DeleteLocalRef(env, docId);
        }
    }

    // Метод 3: Прямой query исходного URI (как в countChildren)
    if (childUri == NULL) {
        LogD("getChildrenList: Using direct URI query as fallback");
        childUri = (*env)->NewLocalRef(env, uri);
        childUriNeedsCleanup = JNI_TRUE;
    }

    if (childUri == NULL) {
        LogD("getChildrenList: All methods failed to build child URI");
        goto cleanup;
    }

    // Выполнение запроса - УПРОЩАЕМ как в countChildren
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

    // ВАЖНО: Используем NULL для проекции как в countChildren, или правильные имена столбцов
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("getChildrenList: Failed to find String class");
        goto cleanup;
    }

    // Правильная проекция с правильными именами столбцов
    jobjectArray projection = (*env)->NewObjectArray(env, 2, stringClass, NULL);
    if (projection == NULL) {
        LogD("getChildrenList: Failed to create projection array");
        goto cleanup;
    }

    // ИСПРАВЛЕНИЕ: Правильные имена столбцов
    jstring colDocumentId = (*env)->NewStringUTF(env, "document_id");
    jstring colDisplayName = (*env)->NewStringUTF(env, "_display_name");

    (*env)->SetObjectArrayElement(env, projection, 0, colDocumentId);
    (*env)->SetObjectArrayElement(env, projection, 1, colDisplayName);

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, projection, NULL, NULL, NULL);

    // Освобождаем ресурсы проекции сразу после использования
    (*env)->DeleteLocalRef(env, colDocumentId);
    (*env)->DeleteLocalRef(env, colDisplayName);
    (*env)->DeleteLocalRef(env, projection);
    (*env)->DeleteLocalRef(env, stringClass);

    if (caseException(env, "query for children list")) {
        LogD("getChildrenList: Query failed with exception");
        goto cleanup;
    }
    if (cursor == NULL) {
        LogD("getChildrenList: Query returned NULL cursor");
        goto cleanup;
    }

    // Остальная часть кода обработки курсора остается без изменений
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

    // Получаем индексы столбцов
    jstring displayNameColumn = (*env)->NewStringUTF(env, "_display_name");
    jstring documentIdColumn = (*env)->NewStringUTF(env, "document_id");

    jint colDisplayNameIndex = (*env)->CallIntMethod(env, cursor, getColumnIndex, displayNameColumn);
    jint colDocumentIdIndex = (*env)->CallIntMethod(env, cursor, getColumnIndex, documentIdColumn);

    (*env)->DeleteLocalRef(env, displayNameColumn);
    (*env)->DeleteLocalRef(env, documentIdColumn);

    // Используем display_name если доступен, иначе document_id
    jint nameColumnIndex = colDisplayNameIndex >= 0 ? colDisplayNameIndex : colDocumentIdIndex;
    if (nameColumnIndex < 0) {
        LogD("getChildrenList: No valid name column found");
        goto cleanup;
    }

    LogD("getChildrenList: Using column index %d for names", nameColumnIndex);

    // Инициализация буфера и чтение данных (остается без изменений)
    currentBuffer = malloc(bufferSize);
    if (currentBuffer == NULL) {
        LogD("getChildrenList: Failed to allocate buffer");
        goto cleanup;
    }
    currentBuffer[0] = '\0';

    jboolean hasData = (*env)->CallBooleanMethod(env, cursor, moveToFirst);
    jint index = 0;
    jboolean firstItem = JNI_TRUE;

    while (hasData && index < count) {
        jstring itemName = (jstring)(*env)->CallObjectMethod(env, cursor, getString, nameColumnIndex);

        if (itemName != NULL) {
            const char* nameStr = (*env)->GetStringUTFChars(env, itemName, NULL);
            if (nameStr != NULL) {
                size_t nameLen = strlen(nameStr);

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

                if (!firstItem) {
                    strcat(currentBuffer, "|");
                    currentLength++;
                } else {
                    firstItem = JNI_FALSE;
                }

                strcat(currentBuffer, nameStr);
                currentLength += nameLen;

                (*env)->ReleaseStringUTFChars(env, itemName, nameStr);
            }
            (*env)->DeleteLocalRef(env, itemName);
        }

        index++;
        hasData = (*env)->CallBooleanMethod(env, cursor, moveToNext);
    }

    if (currentLength > 0) {
        result = strdup(currentBuffer);
        LogD("getChildrenList: Successfully read %d items, result length: %zu", index, currentLength);
    } else {
        result = strdup("");
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

    return result;
}

static char* getChildrenURI0(JNIEnv* env, jobject activity, const char* uriStr) {
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

    char* result = NULL;
    char* currentBuffer = NULL;
    size_t bufferSize = 2048; // Больше для URI
    size_t currentLength = 0;

    // Инициализация ContentResolver
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
    if (caseException(env, "getContentResolver")) {
        goto cleanup;
    }
    if (contentResolver == NULL) {
        LogD("getChildrenURI: contentResolver is NULL");
        goto cleanup;
    }

    // Парсинг URI
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
    if (caseException(env, "parse URI")) {
        goto cleanup;
    }
    if (uri == NULL) {
        LogD("getChildrenURI: parse returned NULL");
        goto cleanup;
    }

    // Поиск DocumentsContract класса
    documentsContractClass = (*env)->FindClass(env, "android/provider/DocumentsContract");
    if (documentsContractClass == NULL) {
        LogD("getChildrenURI: Failed to find DocumentsContract class");
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
    jmethodID buildDocumentUriUsingTreeMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
        "buildDocumentUriUsingTree", "(Landroid/net/Uri;Ljava/lang/String;)Landroid/net/Uri;");

    // Метод 1: buildChildDocumentsUriUsingTree (основной)
    if (buildChildDocumentsUriUsingTreeMethod != NULL && getTreeDocumentIdMethod != NULL) {
        jstring treeDocId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getTreeDocumentIdMethod, uri);
        if (caseException(env, "getTreeDocumentId")) {
            LogD("getChildrenURI: getTreeDocumentId failed with exception");
        } else if (treeDocId != NULL) {
            childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                buildChildDocumentsUriUsingTreeMethod, uri, treeDocId);
            if (caseException(env, "buildChildDocumentsUriUsingTree")) {
                LogD("getChildrenURI: buildChildDocumentsUriUsingTree failed with exception");
                if (childUri != NULL) {
                    (*env)->DeleteLocalRef(env, childUri);
                    childUri = NULL;
                }
            } else if (childUri != NULL) {
                childUriNeedsCleanup = JNI_TRUE;
                LogD("getChildrenURI: Successfully built child URI using tree method");
            }
            (*env)->DeleteLocalRef(env, treeDocId);
        }
    }

    // Метод 2: buildChildDocumentsUri (альтернативный)
    if (childUri == NULL && buildChildDocumentsUriMethod != NULL && getDocumentIdMethod != NULL) {
        jstring docId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getDocumentIdMethod, uri);
        if (caseException(env, "getDocumentId")) {
            LogD("getChildrenURI: getDocumentId failed with exception");
        } else if (docId != NULL) {
            childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                buildChildDocumentsUriMethod, uri, docId);
            if (caseException(env, "buildChildDocumentsUri")) {
                LogD("getChildrenURI: buildChildDocumentsUri failed with exception");
                if (childUri != NULL) {
                    (*env)->DeleteLocalRef(env, childUri);
                    childUri = NULL;
                }
            } else if (childUri != NULL) {
                childUriNeedsCleanup = JNI_TRUE;
                LogD("getChildrenURI: Successfully built child URI using document method");
            }
            (*env)->DeleteLocalRef(env, docId);
        }
    }

    // Метод 3: Используем DocumentsContract.buildChildDocumentsUriUsingTree с root (fallback)
    if (childUri == NULL) {
        LogD("getChildrenURI: Trying alternative tree method");
        // Для некоторых провайдеров может сработать получение documentId через getDocumentId
        if (getDocumentIdMethod != NULL && buildChildDocumentsUriUsingTreeMethod != NULL) {
            jstring docId = (jstring)(*env)->CallStaticObjectMethod(env, documentsContractClass, getDocumentIdMethod, uri);
            if (!caseException(env, "getDocumentId for fallback") && docId != NULL) {
                childUri = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                    buildChildDocumentsUriUsingTreeMethod, uri, docId);
                if (caseException(env, "buildChildDocumentsUriUsingTree fallback")) {
                    LogD("getChildrenURI: Fallback tree method failed with exception");
                    if (childUri != NULL) {
                        (*env)->DeleteLocalRef(env, childUri);
                        childUri = NULL;
                    }
                } else if (childUri != NULL) {
                    childUriNeedsCleanup = JNI_TRUE;
                    LogD("getChildrenURI: Successfully built child URI using fallback tree method");
                }
                (*env)->DeleteLocalRef(env, docId);
            }
        }
    }

    // Метод 4: Используем DocumentsContract API для прямого запроса детей
    if (childUri == NULL) {
        LogD("getChildrenURI: Using DocumentsContract children query");
        jmethodID getChildrenDocumentsMethod = (*env)->GetStaticMethodID(env, documentsContractClass,
            "getChildrenDocuments", "(Landroid/content/ContentResolver;Landroid/net/Uri;)Landroid/database/Cursor;");

        if (getChildrenDocumentsMethod != NULL) {
            cursor = (*env)->CallStaticObjectMethod(env, documentsContractClass,
                getChildrenDocumentsMethod, contentResolver, uri);
            if (caseException(env, "getChildrenDocuments")) {
                LogD("getChildrenURI: getChildrenDocuments failed with exception");
                if (cursor != NULL) {
                    (*env)->DeleteLocalRef(env, cursor);
                    cursor = NULL;
                }
            } else if (cursor != NULL) {
                LogD("getChildrenURI: Successfully got children using getChildrenDocuments");
                // Пропускаем построение childUri, т.к. курсор уже получен
                goto process_cursor;
            }
        }
    }

    if (childUri == NULL) {
        LogD("getChildrenURI: All methods failed to build child URI");
        goto cleanup;
    }

    // Выполнение запроса через ContentResolver
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

    // Projection - получаем document_id для построения полных URI
    jclass stringClass = (*env)->FindClass(env, "java/lang/String");
    if (stringClass == NULL) {
        LogD("getChildrenURI: Failed to find String class");
        goto cleanup;
    }

    jobjectArray projection = (*env)->NewObjectArray(env, 1, stringClass, NULL);
    if (projection == NULL) {
        LogD("getChildrenURI: Failed to create projection array");
        goto cleanup;
    }

    jstring colDocumentId = (*env)->NewStringUTF(env, "document_id");
    (*env)->SetObjectArrayElement(env, projection, 0, colDocumentId);
    (*env)->DeleteLocalRef(env, colDocumentId);

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, projection, NULL, NULL, NULL);
    if (caseException(env, "query for children list")) {
        LogD("getChildrenURI: Query failed with exception");
        goto cleanup;
    }
    if (cursor == NULL) {
        LogD("getChildrenURI: Query returned NULL cursor");
        goto cleanup;
    }

process_cursor:
    // Получение данных из курсора
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
        result = strdup(""); // Пустая строка
        goto cleanup;
    }

    // Получаем индекс столбца с document_id
    jstring documentIdColumn = (*env)->NewStringUTF(env, "document_id");
    jint colDocumentIdIndex = (*env)->CallIntMethod(env, cursor, getColumnIndex, documentIdColumn);
    (*env)->DeleteLocalRef(env, documentIdColumn);

    if (colDocumentIdIndex < 0) {
        LogD("getChildrenURI: document_id column not found");
        goto cleanup;
    }

    // Получаем метод toString для Uri
    jmethodID uriToStringMethod = (*env)->GetMethodID(env, uriClass, "toString", "()Ljava/lang/String;");
    if (uriToStringMethod == NULL) {
        LogD("getChildrenURI: Failed to get Uri.toString method");
        goto cleanup;
    }

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
            // Строим полный URI для дочернего документа
            jobject childDocumentUri = NULL;

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

            if (childDocumentUri != NULL) {
                // Получаем строковое представление URI
                jstring childUriString = (jstring)(*env)->CallObjectMethod(env, childDocumentUri, uriToStringMethod);
                if (childUriString != NULL) {
                    const char* uriStrValue = (*env)->GetStringUTFChars(env, childUriString, NULL);
                    if (uriStrValue != NULL) {
                        size_t uriLen = strlen(uriStrValue);

                        // Проверяем, достаточно ли места в буфере
                        if (currentLength + uriLen + 2 > bufferSize) {
                            bufferSize = (currentLength + uriLen + 2) * 2;
                            char* newBuffer = realloc(currentBuffer, bufferSize);
                            if (newBuffer == NULL) {
                                (*env)->ReleaseStringUTFChars(env, childUriString, uriStrValue);
                                (*env)->DeleteLocalRef(env, childUriString);
                                (*env)->DeleteLocalRef(env, childDocumentUri);
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
                    }
                    (*env)->DeleteLocalRef(env, childUriString);
                }
                (*env)->DeleteLocalRef(env, childDocumentUri);
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
    // Освобождение памяти буфера
    if (currentBuffer != NULL) {
        free(currentBuffer);
    }

    // Закрытие курсора
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

    return result;
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
func MimeType(uri fyne.URI) (mimeTypeStr string) {
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
			log.Tracef("countChildren: successfully counted %d children for URI: %s", count, uri.String())
		}

		return nil
	})

	return count, err
}

// IsDirectory проверяет, является ли URI каталогом
func IsDirectory(uri fyne.URI) bool {
	// log.Tracef("-------------IsDirectory %s", uri)
	if uri == nil {
		return false
	}
	switch MimeType(uri) {
	case MIME_TYPE_DIR:
		// log.Trace("-------------MIME_TYPE_DIR true")
		return true
	case MIME_TYPE_OCTET_STREAM:
		if strings.HasPrefix(uri.String(), ZhangHai) {
			size, sizeErr := getSize(uri)
			if sizeErr == nil && size == 4096 {
				// log.Trace("-------------ZhangHai true")
				return true
			}
		}
		fallthrough
	case "":
		// отсутствуют права
		_, err := countChild(uri)
		// log.Tracef("-------------countChild %d error: %v %v", c, err, err == nil)
		return err == nil
	default:
		// log.Trace("-------------false")
		return false
	}
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
			log.Tracef("getChildrenList: successfully got children string for URI: %s", uri.String())
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

	log.Tracef("getChildrenList: parsed %d children for URI: %s", len(children), uri.String())

	return children, nil
}

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
			log.Tracef("getChildrenURI: successfully got children URI string for URI: %s", uri.String())
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

	log.Tracef("getChildrenURI: parsed %d children URIs for parent URI: %s", len(children), uri.String())

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

func Reader(u fyne.URI) (r fyne.URIReadCloser, err error) {
	if u == nil {
		err = fmt.Errorf("uri is nul")
		return
	}
	if u.Scheme() == "content" {
		return reader(u) //  84434413 ~2.23 сек быстрее на 34%
	}
	if !canRead(u) {
		err = fmt.Errorf("uri not readable")
		return
	}
	return storage.Reader(u) // 84434413 ~3.38 сек
}

func canRead(uri fyne.URI) bool {
	if uri == nil {
		return false
	}
	switch MimeType(uri) {
	case MIME_TYPE_DIR:
		return false
	case MIME_TYPE_OCTET_STREAM:
		if strings.HasPrefix(uri.String(), ZhangHai) {
			size, sizeErr := getSize(uri)
			if sizeErr == nil && size == 4096 {
				return false // иначе storage.CanRead  вернёт syscall.EISDIR и крэшит
			}
		}
	}
	ok, err := storage.CanRead(uri)
	if err != nil {
		log.Errorf("canRead: %w", err)
		return false
	}
	if !ok {
		return false
	}
	log.Trace("CanRead %s", uri)

	r, err := storage.Reader(uri)
	if err != nil {
		log.Errorf("reader: %w", err)
		return false
	}
	defer r.Close()

	p := make([]byte, 1)
	_, err = r.Read(p)
	if err != nil {
		log.Errorf("read: %w", err)
		return false
	}
	return true
}
