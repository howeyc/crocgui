//go:build android

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

// checkForSelfReference проверяет циклические ссылки и возвращает количество self-reference
static jint checkForSelfReference(JNIEnv* env, jobject contentResolver, jobject parentUri, jobject childUri) {
    jobject cursor = NULL;
    jclass cursorClass = NULL;
    jclass uriClass = NULL;
    jint selfReferenceCount = 0;

    if (contentResolver == NULL || childUri == NULL || parentUri == NULL) {
        return 0;
    }

    LogD("checkForSelfReference: Checking children for self-reference");

    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (uriClass == NULL) {
        return 0;
    }

    jclass resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (resolverClass == NULL) {
        (*env)->DeleteLocalRef(env, uriClass);
        return 0;
    }

    jmethodID queryMethod = (*env)->GetMethodID(env, resolverClass, "query",
        "(Landroid/net/Uri;[Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;Ljava/lang/String;)Landroid/database/Cursor;");
    if (queryMethod == NULL) {
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, resolverClass);
        return 0;
    }

    cursor = (*env)->CallObjectMethod(env, contentResolver, queryMethod, childUri, NULL, NULL, NULL, NULL);
    if (caseException(env, "query for self-reference check") || cursor == NULL) {
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, resolverClass);
        return 0;
    }

    cursorClass = (*env)->GetObjectClass(env, cursor);
    if (cursorClass == NULL) {
        (*env)->DeleteLocalRef(env, cursor);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, resolverClass);
        return 0;
    }

    jmethodID moveToFirst = (*env)->GetMethodID(env, cursorClass, "moveToFirst", "()Z");
    jmethodID moveToNext = (*env)->GetMethodID(env, cursorClass, "moveToNext", "()Z");
    jmethodID getString = (*env)->GetMethodID(env, cursorClass, "getString", "(I)Ljava/lang/String;");

    if (moveToFirst == NULL || moveToNext == NULL || getString == NULL) {
        (*env)->DeleteLocalRef(env, cursor);
        (*env)->DeleteLocalRef(env, cursorClass);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, resolverClass);
        return 0;
    }

    jmethodID uriToString = (*env)->GetMethodID(env, uriClass, "toString", "()Ljava/lang/String;");
    if (uriToString == NULL) {
        (*env)->DeleteLocalRef(env, cursor);
        (*env)->DeleteLocalRef(env, cursorClass);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, resolverClass);
        return 0;
    }

    jstring parentUriStr = (*env)->CallObjectMethod(env, parentUri, uriToString);
    if (caseException(env, "parent URI to string") || parentUriStr == NULL) {
        (*env)->DeleteLocalRef(env, cursor);
        (*env)->DeleteLocalRef(env, cursorClass);
        (*env)->DeleteLocalRef(env, uriClass);
        (*env)->DeleteLocalRef(env, resolverClass);
        return 0;
    }

    // Проходим по всем дочерним элементам и считаем self-reference
    if ((*env)->CallBooleanMethod(env, cursor, moveToFirst)) {
        do {
            jstring childDocumentUriStr = (jstring)(*env)->CallObjectMethod(env, cursor, getString, 0);
            if (caseException(env, "get child document URI") || childDocumentUriStr == NULL) {
                continue;
            }

            const char* parentStr = (*env)->GetStringUTFChars(env, parentUriStr, NULL);
            const char* childStr = (*env)->GetStringUTFChars(env, childDocumentUriStr, NULL);

            if (parentStr != NULL && childStr != NULL && strcmp(parentStr, childStr) == 0) {
                selfReferenceCount++;
                LogD("checkForSelfReference: Found self-reference [%d]: %s", selfReferenceCount, parentStr);
            }

            if (parentStr != NULL) {
                (*env)->ReleaseStringUTFChars(env, parentUriStr, parentStr);
            }
            if (childStr != NULL) {
                (*env)->ReleaseStringUTFChars(env, childDocumentUriStr, childStr);
            }

            (*env)->DeleteLocalRef(env, childDocumentUriStr);

        } while ((*env)->CallBooleanMethod(env, cursor, moveToNext));
    }

    LogD("checkForSelfReference: Found %d self-references", selfReferenceCount);

    // Очистка ресурсов
    (*env)->DeleteLocalRef(env, parentUriStr);
    if (cursor) {
        jmethodID closeMethod = (*env)->GetMethodID(env, cursorClass, "close", "()V");
        if (closeMethod != NULL) {
            (*env)->CallVoidMethod(env, cursor, closeMethod);
            caseException(env, "close cursor in self-reference check");
        }
        (*env)->DeleteLocalRef(env, cursor);
    }
    (*env)->DeleteLocalRef(env, cursorClass);
    (*env)->DeleteLocalRef(env, uriClass);
    (*env)->DeleteLocalRef(env, resolverClass);

    return selfReferenceCount;
}

// countChildren возвращает количество дочерних элементов (исправленная версия)
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

                            // Проверяем на циклические ссылки и корректируем count
                            if (count > 0) {
                                jint selfReferenceCount = checkForSelfReference(env, contentResolver, uri, childUri);

                                if (selfReferenceCount > 0) {
                                    jint originalCount = count;
                                    count = count - selfReferenceCount;
                                    LogD("countChildren: Adjusted count from %d to %d (removed %d self-references)",
                                         originalCount, count, selfReferenceCount);

                                    // Защита от отрицательных значений
                                    if (count < 0) {
                                        LogD("countChildren: Warning: adjusted count is negative (%d), setting to 0", count);
                                        count = 0;
                                    }
                                }
                            }
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
	"strings"
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

// countChild возвращает количество дочерних элементов в директории
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

		// Обрабатываем коды ошибок
		if count < 0 {
			switch count {
			case -1:
				err = fmt.Errorf("general failure")
			case -3:
				err = fmt.Errorf("cursor methods not available")
			case -4:
				err = fmt.Errorf("cursor class not available")
			case -5:
				err = fmt.Errorf("query returned NULL cursor")
			case -6:
				err = fmt.Errorf("query method not available")
			case -7:
				err = fmt.Errorf("failed to build child URI")
			case -8:
				err = fmt.Errorf("query failed with exception")
			case -9:
				err = fmt.Errorf("getCount failed with exception")
			default:
				err = fmt.Errorf("unknown error code: %d", count)
			}
			count = 0
		}

		return nil
	})

	return count, err
}

// IsDirectory проверяет, является ли URI директорией (чистая Go реализация)
func IsDirectory(uri fyne.URI) bool {
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
	if size == 4096 && mime == "application/octet-stream" && strings.HasPrefix(uri.String(), "content://me.zhanghai.android.files.file_provider/") {
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
		strings.HasPrefix(uri.String(), "content://me.zhanghai.android.files.file_provider/") {
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
