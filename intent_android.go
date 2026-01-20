//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <android/log.h>
#include <sys/stat.h>
#include <errno.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)
static const jint RESULT_OK = -1;
static const jint RESULT_CANCELED = 0;

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

// receiveURIFromIntent и receiveTextFromIntent для обработки интентов
void receiveURIFromIntent(char* uri);
void receiveTextFromIntent(char* text);

// Функция для установки результата активности
static void setResult(JNIEnv* env, jobject activity, jint resultCode) {
    jclass activity_class = NULL;
    jmethodID setResult = NULL;

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for setResult");
        return;
    }

    setResult = (*env)->GetMethodID(env, activity_class, "setResult", "(I)V");
    if (setResult == NULL) {
        LogD("C: ERROR - Failed to get setResult method");
        goto cleanup;
    }

    LogD("C: Setting result: %d", resultCode);

    (*env)->CallVoidMethod(env, activity, setResult, resultCode);

cleanup:
    if (activity_class) {
        (*env)->DeleteLocalRef(env, activity_class);
    }
}

// Функция для завершения активности
static void finish(JNIEnv* env, jobject activity) {
    jclass activity_class = NULL;
    jmethodID finishMethod = NULL;

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for finish");
        return;
    }

    finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
    if (finishMethod == NULL) {
        LogD("C: ERROR - Failed to get finish method");
        goto cleanup;
    }

    LogD("C: Finishing activity");
    (*env)->CallVoidMethod(env, activity, finishMethod);

cleanup:
    if (activity_class) {
        (*env)->DeleteLocalRef(env, activity_class);
    }
}

// Функция для исключения активности из недавних приложений
static void excludeFromRecents(JNIEnv* env, jobject activity) {
    jclass activity_class = NULL;
    jmethodID finishAndRemoveTask = NULL;
    jmethodID finishAffinity = NULL;
    jmethodID finishMethod = NULL;

    jclass buildVersionClass = NULL;
    jfieldID sdkIntField = NULL;
    jint sdkInt = 0;

    // Получаем версию Android SDK
    buildVersionClass = (*env)->FindClass(env, "android/os/Build$VERSION");
    if (buildVersionClass != NULL) {
        sdkIntField = (*env)->GetStaticFieldID(env, buildVersionClass, "SDK_INT", "I");
        if (sdkIntField != NULL) {
            sdkInt = (*env)->GetStaticIntField(env, buildVersionClass, sdkIntField);
        }
        (*env)->DeleteLocalRef(env, buildVersionClass);
    }

    LogD("C: Android SDK: %d", sdkInt);

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for excludeFromRecents");
        return;
    }

    if (sdkInt >= 29) {
        // Android 10+ - используем finishAndRemoveTask
        finishAndRemoveTask = (*env)->GetMethodID(env, activity_class, "finishAndRemoveTask", "()V");
        if (finishAndRemoveTask != NULL) {
            LogD("C: Using finishAndRemoveTask (Android 10+)");

            // Безопасный вызов с обработкой исключений
            (*env)->CallVoidMethod(env, activity, finishAndRemoveTask);
            if (caseException(env, "finishAndRemoveTask")) {
                LogD("C: finishAndRemoveTask failed, falling back to finish()");
                // Fallback
                finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
                if (finishMethod != NULL) {
                    (*env)->CallVoidMethod(env, activity, finishMethod);
                    caseException(env, "finish fallback");
                }
            }
        } else {
            LogD("C: finishAndRemoveTask not available");
            // Fallback to finish()
            finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
            if (finishMethod != NULL) {
                (*env)->CallVoidMethod(env, activity, finishMethod);
                caseException(env, "finish");
            }
        }
    } else {
        // Android 9 и ниже
        LogD("C: Using finish() for Android < 10");
        finishMethod = (*env)->GetMethodID(env, activity_class, "finish", "()V");
        if (finishMethod != NULL) {
            (*env)->CallVoidMethod(env, activity, finishMethod);
            caseException(env, "finish for Android < 10");
        } else {
            LogD("C: ERROR - No finish method found!");
        }
    }

cleanup:
    if (activity_class) {
        (*env)->DeleteLocalRef(env, activity_class);
    }
}

static void clearIntent(JNIEnv* env, jobject activity) {
    jclass activity_class = NULL;
    jmethodID get_intent = NULL;
    jmethodID set_intent = NULL;
    jobject intent = NULL;
    jclass intentClass = NULL;

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for clearIntent");
        return;
    }

    // Получаем текущий Intent
    get_intent = (*env)->GetMethodID(env, activity_class, "getIntent", "()Landroid/content/Intent;");
    if (get_intent == NULL) {
        LogD("C: ERROR - Failed to get getIntent method");
        goto cleanup;
    }

    intent = (*env)->CallObjectMethod(env, activity, get_intent);
    if (intent == NULL) {
        LogD("C: Intent is NULL in clearIntent");
        goto cleanup;
    }

    // Создаем новый пустой Intent
    intentClass = (*env)->FindClass(env, "android/content/Intent");
    if (intentClass == NULL) {
        LogD("C: ERROR - Failed to find Intent class");
        goto cleanup;
    }

    jobject newIntent = (*env)->NewObject(env, intentClass,
        (*env)->GetMethodID(env, intentClass, "<init>", "()V"));

    if (newIntent == NULL) {
        LogD("C: ERROR - Failed to create new Intent");
        goto cleanup;
    }

    // Устанавливаем новый Intent
    set_intent = (*env)->GetMethodID(env, activity_class, "setIntent", "(Landroid/content/Intent;)V");
    if (set_intent != NULL) {
        (*env)->CallVoidMethod(env, activity, set_intent, newIntent);
        LogD("C: Intent cleared and replaced with empty one");
    }

    (*env)->DeleteLocalRef(env, newIntent);

cleanup:
    if (intentClass) {
        (*env)->DeleteLocalRef(env, intentClass);
    }
    if (intent) {
        (*env)->DeleteLocalRef(env, intent);
    }
    if (activity_class) {
        (*env)->DeleteLocalRef(env, activity_class);
    }
}

static void processIntent(JNIEnv* env, jobject activity) {
    // Получаем класс активности
    jclass activity_class = NULL;
    jmethodID get_intent = NULL;
    jobject intent = NULL;
    jclass intent_class = NULL;
    jmethodID getAction = NULL;
    jstring action = NULL;
    const char *actionStr = NULL;
    int isSend = 0;
    int isView = 0;
    int isSendMultiple = 0;
    int isMain = 0;
    int hasValidData = 0;

    activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class");
        goto cleanup;
    }

    // Получаем метод getIntent
    get_intent = (*env)->GetMethodID(env, activity_class, "getIntent", "()Landroid/content/Intent;");
    if (get_intent == NULL) {
        LogD("C: ERROR - Failed to get getIntent method");
        goto cleanup;
    }

    // Вызываем getIntent()
    intent = (*env)->CallObjectMethod(env, activity, get_intent);
    if (intent == NULL) {
        LogD("C: ERROR - Intent is NULL");
        receiveTextFromIntent(strdup(""));
        goto cleanup;
    }

    // Получаем класс Intent
    intent_class = (*env)->GetObjectClass(env, intent);
    if (intent_class == NULL) {
        LogD("C: ERROR - Failed to get intent class");
        goto cleanup;
    }

    jmethodID getFlags = (*env)->GetMethodID(env, intent_class, "getFlags", "()I");
    if (getFlags != NULL) {
        jint flags = (*env)->CallIntMethod(env, intent, getFlags);
        LogD("C: INTENT FLAGS: 0x%x", flags);

        if (flags & 0x10000000) LogD("C: [0x10000000] NEW_TASK");

        if (flags & 0x08000000) LogD("C: [0x08000000] SINGLE_TOP");
        if (flags & 0x04000000) LogD("C: [0x04000000] CLEAR_TOP");
        if (flags & 0x02000000) LogD("C: [0x02000000] FORWARD_RESULT");
        if (flags & 0x01000000) LogD("C: [0x01000000] PREVIOUS_IS_TOP");

        if (flags & 0x00800000) LogD("C: [0x00800000] EXCLUDE_FROM_RECENTS");
        if (flags & 0x00400000) LogD("C: [0x00400000] NO_HISTORY");
        if (flags & 0x00200000) LogD("C: [0x00200000] RESET_TASK_IF_NEEDED");
        if (flags & 0x00100000) LogD("C: [0x00100000] LAUNCHED_FROM_HISTORY");

        if (flags & 0x00080000) LogD("C: [0x00080000] CLEAR_TASK");
        if (flags & 0x00040000) LogD("C: [0x00040000] MULTIPLE_TASK");
        if (flags & 0x00020000) LogD("C: [0x00020000] REORDER_TO_FRONT");
        if (flags & 0x00010000) LogD("C: [0x00010000] NO_ANIMATION");

        if (flags & 0x00008000) LogD("C: [0x00008000] TASK_ON_HOME");
        if (flags & 0x00004000) LogD("C: [0x00004000] NO_USER_ACTION");
        if (flags & 0x00002000) LogD("C: [0x00002000] BROUGHT_TO_FRONT");
        if (flags & 0x00001000) LogD("C: [0x00001000] RECEIVER_REGISTERED_ONLY");

        if (flags & 0x00000080) LogD("C: [0x00000080] RETAIN_IN_RECENTS");
        if (flags & 0x00000040) LogD("C: [0x00000040] LAUNCH_ADJACENT");
        if (flags & 0x00000020) LogD("C: [0x00000020] MATCH_EXTERNAL");
        if (flags & 0x00000010) LogD("C: [0x00000010] REQUIRE_NON_BROWSER");

        if (flags & 0x00000800) LogD("C: [0x00000800] RELAUNCHED_FROM_HISTORY");
        if (flags & 0x00000400) LogD("C: [0x00000400] FLAG_RECEIVER_NO_ABORT");
        if (flags & 0x00000200) LogD("C: [0x00000200] RECEIVER_VISIBLE_TO_INSTANT_APPS");
        if (flags & 0x00000100) LogD("C: [0x00000100] DIRECT_BOOT_AUTO");

        if (flags & 0x00000008) LogD("C: [0x00000008] REQUIRE_DEFAULT");
        if (flags & 0x00000004) LogD("C: [0x00000004] RECEIVER_NO_ABORT");
        if (flags & 0x00000002) LogD("C: [0x00000002] RECEIVER_REGISTERED_ONLY_BEFORE_BOOT");
        if (flags & 0x00000001) LogD("C: [0x00000001] RECEIVER_REPLACE_PENDING");

        if (flags & 0x00100000) { // LAUNCHED_FROM_HISTORY
            LogD("C: Skipping: Activity launched from history");
            goto cleanup;
        }
        if (flags & 0x00002000) { // BROUGHT_TO_FRONT
            LogD("C: Skipping: Activity brought to front");
            goto cleanup;
        }

        if (flags & 0x00000800) { // RELAUNCHED_FROM_HISTORY
            LogD("C: Skipping: Activity relaunched from history (Android 15+)");
            goto cleanup;
        }
    }

    // Получаем Action интента
    getAction = (*env)->GetMethodID(env, intent_class, "getAction", "()Ljava/lang/String;");
    if (getAction != NULL) {
        action = (*env)->CallObjectMethod(env, intent, getAction);
        if (action != NULL) {
            actionStr = (*env)->GetStringUTFChars(env, action, NULL);
            if (actionStr != NULL) {
                LogD("C: Intent action: %s", actionStr);

                isSend = strcmp(actionStr, "android.intent.action.SEND") == 0;
                isView = strcmp(actionStr, "android.intent.action.VIEW") == 0;
                isSendMultiple = strcmp(actionStr, "android.intent.action.SEND_MULTIPLE") == 0;
                isMain = strcmp(actionStr, "android.intent.action.MAIN") == 0;

                (*env)->ReleaseStringUTFChars(env, action, actionStr);
            }
            (*env)->DeleteLocalRef(env, action);
        } else {
            LogD("C: Intent action is NULL");
            receiveTextFromIntent(strdup(""));
            goto cleanup;
        }
    }

    if (isMain) {
        LogD("C: MAIN intent - starting main app");
        goto cleanup;
    }

    // First check ClipData
    jmethodID getClipData = (*env)->GetMethodID(env, intent_class, "getClipData", "()Landroid/content/ClipData;");
    if (getClipData != NULL) {
        jobject clipData = NULL;

        clipData = (*env)->CallObjectMethod(env, intent, getClipData);

        if (clipData != NULL) {
            LogD("C: ClipData found! Processing...");

            jclass clipData_class = NULL;
            jmethodID getItemCount = NULL;
            jmethodID getItemAt = NULL;

            clipData_class = (*env)->GetObjectClass(env, clipData);
            if (clipData_class != NULL) {
                getItemCount = (*env)->GetMethodID(env, clipData_class, "getItemCount", "()I");
                getItemAt = (*env)->GetMethodID(env, clipData_class, "getItemAt", "(I)Landroid/content/ClipData$Item;");

                if (getItemCount != NULL && getItemAt != NULL) {
                    jint itemCount = (*env)->CallIntMethod(env, clipData, getItemCount);
                    LogD("C: ClipData item count: %d", itemCount);

                    for (int i = 0; i < itemCount; i++) {
                        LogD("C: Processing item %d", i);

                        jobject item = (*env)->CallObjectMethod(env, clipData, getItemAt, i);
                        if (item != NULL) {
                            jclass item_class = NULL;

                            item_class = (*env)->GetObjectClass(env, item);

                            if (item_class != NULL) {
                                // Process URI
                                jmethodID getUri = (*env)->GetMethodID(env, item_class, "getUri", "()Landroid/net/Uri;");
                                if (getUri != NULL) {
                                    jobject uri = (*env)->CallObjectMethod(env, item, getUri);
                                    if (uri != NULL) {
                                        LogD("C: Found URI in ClipData item");
                                        jclass uri_class = NULL;
                                        jmethodID to_string = NULL;

                                        uri_class = (*env)->GetObjectClass(env, uri);
                                        if (uri_class != NULL) {
                                            to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                                            if (to_string != NULL) {
                                                jstring uri_string = (*env)->CallObjectMethod(env, uri, to_string);
                                                if (uri_string != NULL) {
                                                    const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                                                    if (utf_str != NULL) {
                                                        LogD("C: Sending URI to Go: %s", utf_str);
                                                        receiveURIFromIntent(strdup(utf_str));
                                                        hasValidData = 1;
                                                        (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                                                    }
                                                    (*env)->DeleteLocalRef(env, uri_string);
                                                }
                                            }
                                            (*env)->DeleteLocalRef(env, uri_class);
                                        }
                                        (*env)->DeleteLocalRef(env, uri);
                                    }
                                }

                                // Process text
                                jmethodID getText = (*env)->GetMethodID(env, item_class, "getText", "()Ljava/lang/CharSequence;");
                                if (getText != NULL) {
                                    jobject text = (*env)->CallObjectMethod(env, item, getText);
                                    if (text != NULL) {
                                        LogD("C: Found text in ClipData item");
                                        jclass text_class = NULL;
                                        jmethodID toString = NULL;

                                        text_class = (*env)->GetObjectClass(env, text);
                                        if (text_class != NULL) {
                                            toString = (*env)->GetMethodID(env, text_class, "toString", "()Ljava/lang/String;");
                                            if (toString != NULL) {
                                                jstring text_string = (*env)->CallObjectMethod(env, text, toString);
                                                if (text_string != NULL) {
                                                    const char *text_str = (*env)->GetStringUTFChars(env, text_string, NULL);
                                                    if (text_str != NULL) {
                                                        if (strlen(text_str) > 100) {
                                                            LogD("C: Sending text to Go (truncated): %.100s...", text_str);
                                                        } else {
                                                            LogD("C: Sending text to Go: %s", text_str);
                                                        }
                                                        receiveTextFromIntent(strdup(text_str));
                                                        hasValidData = 1;
                                                        (*env)->ReleaseStringUTFChars(env, text_string, text_str);
                                                    }
                                                    (*env)->DeleteLocalRef(env, text_string);
                                                }
                                            }
                                            (*env)->DeleteLocalRef(env, text_class);
                                        }
                                        (*env)->DeleteLocalRef(env, text);
                                    }
                                }
                                (*env)->DeleteLocalRef(env, item_class);
                            }
                            (*env)->DeleteLocalRef(env, item);
                        }
                    }
                }
                (*env)->DeleteLocalRef(env, clipData_class);
            }
            (*env)->DeleteLocalRef(env, clipData);

            // clearIntent(env, activity);
            if (hasValidData) {
                setResult(env, activity, RESULT_OK);
                LogD("C: ClipData processing complete - setting RESULT_OK");
                // jmethodID setAction = (*env)->GetMethodID(env, intent_class, "setAction", "(Ljava/lang/String;)Landroid/content/Intent;");
                // if (setAction != NULL) {
                //     (*env)->CallObjectMethod(env, intent, setAction, NULL);
                //     LogD("C: Intent action cleared to prevent duplication");
                // }
            } else {
                setResult(env, activity, RESULT_CANCELED);
                LogD("C: ClipData processing complete - no valid data, setting RESULT_CANCELED");
            }

            goto cleanup;
        } else {
            LogD("C: No ClipData found");
        }
    } else {
        LogD("C: getClipData method not available");
    }

    // Затем проверяем ACTION_SEND text/plain
    if (isSend) {
        LogD("C: Checking for SEND text/plain...");
        jmethodID getType = NULL;

        getType = (*env)->GetMethodID(env, intent_class, "getType", "()Ljava/lang/String;");
        if (getType != NULL) {
            jstring type = (*env)->CallObjectMethod(env, intent, getType);
            if (type != NULL) {
                const char *typeStr = (*env)->GetStringUTFChars(env, type, NULL);
                if (typeStr != NULL) {
                    LogD("C: Intent type: %s", typeStr);

                    int isTextPlain = strcmp(typeStr, "text/plain") == 0;
                    (*env)->ReleaseStringUTFChars(env, type, typeStr);

                    if (isTextPlain) {
                        LogD("C: Processing text/plain SEND intent");
                        jmethodID getStringExtra = NULL;

                        getStringExtra = (*env)->GetMethodID(env, intent_class,
                            "getStringExtra", "(Ljava/lang/String;)Ljava/lang/String;");
                        if (getStringExtra != NULL) {
                            jstring extraKey = NULL;
                            jstring text = NULL;

                            extraKey = (*env)->NewStringUTF(env, "android.intent.extra.TEXT");
                            text = (*env)->CallObjectMethod(env, intent, getStringExtra, extraKey);
                            (*env)->DeleteLocalRef(env, extraKey);

                            if (text != NULL) {
                                const char *textStr = (*env)->GetStringUTFChars(env, text, NULL);
                                if (textStr != NULL) {
                                    if (strlen(textStr) > 100) {
                                        LogD("C: Sending SEND text to Go: %.100s...", textStr);
                                    } else {
                                        LogD("C: Sending SEND text to Go: %s", textStr);
                                    }
                                    receiveTextFromIntent(strdup(textStr));
                                    setResult(env, activity, RESULT_OK);
                                    LogD("C: SEND text processing complete - setting RESULT_OK");
                                    (*env)->ReleaseStringUTFChars(env, text, textStr);
                                }
                                (*env)->DeleteLocalRef(env, text);

                                (*env)->DeleteLocalRef(env, type);
                                goto cleanup;
                            }
                        }
                    }
                }
                (*env)->DeleteLocalRef(env, type);
            }
        }
    }

    // Handle ACTION_VIEW (single URI)
    if (isView) {
        LogD("C: Checking for VIEW URI...");
        jmethodID get_data = NULL;

        get_data = (*env)->GetMethodID(env, intent_class, "getData", "()Landroid/net/Uri;");
        if (get_data != NULL) {
            jobject uri = (*env)->CallObjectMethod(env, intent, get_data);
            if (uri != NULL) {
                LogD("C: Found URI in VIEW intent");
                jclass uri_class = NULL;
                jmethodID to_string = NULL;

                uri_class = (*env)->GetObjectClass(env, uri);
                if (uri_class != NULL) {
                    to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                    if (to_string != NULL) {
                        jstring uri_string = (*env)->CallObjectMethod(env, uri, to_string);
                        if (uri_string != NULL) {
                            const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                            if (utf_str != NULL) {
                                LogD("C: Sending VIEW URI to Go: %s", utf_str);
                                receiveURIFromIntent(strdup(utf_str));
                                setResult(env, activity, RESULT_OK);
                                LogD("C: VIEW URI processing complete - setting RESULT_OK");
                                (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                            }
                            (*env)->DeleteLocalRef(env, uri_string);
                        }
                    }
                    (*env)->DeleteLocalRef(env, uri_class);
                }

                (*env)->DeleteLocalRef(env, uri);
                goto cleanup;
            }
        }
    }

    // Handle ACTION_SEND (single content)
    if (isSend) {
        LogD("C: Checking for SEND stream...");
        jmethodID get_extra = NULL;

        get_extra = (*env)->GetMethodID(env, intent_class, "getParcelableExtra", "(Ljava/lang/String;)Landroid/os/Parcelable;");
        if (get_extra != NULL) {
            jstring extra_key = NULL;
            jobject send_uri = NULL;

            extra_key = (*env)->NewStringUTF(env, "android.intent.extra.STREAM");
            send_uri = (*env)->CallObjectMethod(env, intent, get_extra, extra_key);
            (*env)->DeleteLocalRef(env, extra_key);

            if (send_uri != NULL) {
                LogD("C: Found stream URI in SEND intent");
                jclass uri_class = NULL;
                jmethodID to_string = NULL;

                uri_class = (*env)->GetObjectClass(env, send_uri);
                if (uri_class != NULL) {
                    to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                    if (to_string != NULL) {
                        jstring uri_string = (*env)->CallObjectMethod(env, send_uri, to_string);
                        if (uri_string != NULL) {
                            const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                            if (utf_str != NULL) {
                                LogD("C: Sending SEND stream URI to Go: %s", utf_str);
                                receiveURIFromIntent(strdup(utf_str));
                                setResult(env, activity, RESULT_OK);
                                LogD("C: SEND stream processing complete - setting RESULT_OK");
                                (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                            }
                            (*env)->DeleteLocalRef(env, uri_string);
                        }
                    }
                    (*env)->DeleteLocalRef(env, uri_class);
                }

                (*env)->DeleteLocalRef(env, send_uri);
                goto cleanup;
            }
        }
    }

    // Handle ACTION_SEND_MULTIPLE (multiple content)
    if (isSendMultiple) {
        LogD("C: Checking for SEND_MULTIPLE...");
        jmethodID get_array = NULL;

        get_array = (*env)->GetMethodID(env, intent_class, "getParcelableArrayListExtra", "(Ljava/lang/String;)Ljava/util/ArrayList;");
        if (get_array != NULL) {
            jstring array_key = NULL;
            jobject uri_list = NULL;

            array_key = (*env)->NewStringUTF(env, "android.intent.extra.STREAM");
            uri_list = (*env)->CallObjectMethod(env, intent, get_array, array_key);
            (*env)->DeleteLocalRef(env, array_key);

            if (uri_list != NULL) {
                LogD("C: Found URI list in SEND_MULTIPLE intent");
                jclass array_list_class = NULL;
                jmethodID get_size = NULL;
                jmethodID get_item = NULL;

                array_list_class = (*env)->GetObjectClass(env, uri_list);
                if (array_list_class != NULL) {
                    get_size = (*env)->GetMethodID(env, array_list_class, "size", "()I");
                    get_item = (*env)->GetMethodID(env, array_list_class, "get", "(I)Ljava/lang/Object;");

                    if (get_size != NULL && get_item != NULL) {
                        jint size = (*env)->CallIntMethod(env, uri_list, get_size);
                        LogD("C: SEND_MULTIPLE URI count: %d", size);

                        for (int i = 0; i < size; i++) {
                            LogD("C: Processing URI %d", i);

                            jobject current_uri = (*env)->CallObjectMethod(env, uri_list, get_item, i);
                            if (current_uri != NULL) {
                                jclass uri_class = NULL;
                                jmethodID to_string = NULL;

                                uri_class = (*env)->GetObjectClass(env, current_uri);
                                if (uri_class != NULL) {
                                    to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                                    if (to_string != NULL) {
                                        jstring uri_string = (*env)->CallObjectMethod(env, current_uri, to_string);
                                        if (uri_string != NULL) {
                                            const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                                            if (utf_str != NULL) {
                                                LogD("C: Sending SEND_MULTIPLE URI to Go: %s", utf_str);
                                                receiveURIFromIntent(strdup(utf_str));
                                                hasValidData = 1;
                                                (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                                            }
                                            (*env)->DeleteLocalRef(env, uri_string);
                                        }
                                    }
                                    (*env)->DeleteLocalRef(env, uri_class);
                                }
                                (*env)->DeleteLocalRef(env, current_uri);
                            }
                        }

                        // clearIntent(env, activity);
                        if (hasValidData) {
                            setResult(env, activity, RESULT_OK);
                            LogD("C: SEND_MULTIPLE processing complete - setting RESULT_OK");
                            // jmethodID setAction = (*env)->GetMethodID(env, intent_class, "setAction", "(Ljava/lang/String;)Landroid/content/Intent;");
                            // if (setAction != NULL) {
                            //     (*env)->CallObjectMethod(env, intent, setAction, NULL);
                            //     LogD("C: Intent action cleared to prevent duplication");
                            // }
                        } else {
                            setResult(env, activity, RESULT_CANCELED);
                            LogD("C: SEND_MULTIPLE processing complete - no valid data, setting RESULT_CANCELED");
                        }
                    }
                    (*env)->DeleteLocalRef(env, array_list_class);
                }
                (*env)->DeleteLocalRef(env, uri_list);
                goto cleanup;
            }
        }
    }

    // Если дошли сюда - данные не найдены
    LogD("C: No matching intent data found");
    setResult(env, activity, RESULT_CANCELED);

cleanup:
    if (activity_class) {
        (*env)->DeleteLocalRef(env, activity_class);
    }
    if (intent_class) {
        (*env)->DeleteLocalRef(env, intent_class);
    }
    if (intent) {
        (*env)->DeleteLocalRef(env, intent);
    }
}

// Упрощенная версия - всегда использует класс текущей активности
static void startActivitySimple(JNIEnv* env, jobject activity) {
    jclass activityClass = NULL;
    jclass intentClass = NULL;
    jobject intent = NULL;

    activityClass = (*env)->GetObjectClass(env, activity);
    if (activityClass == NULL) {
        LogD("C: ERROR - Failed to get activity class");
        return;
    }

    intentClass = (*env)->FindClass(env, "android/content/Intent");
    if (intentClass == NULL) {
        LogD("C: ERROR - Failed to find Intent class");
        goto cleanup_activity;
    }

    // Создаем Intent для текущего класса активности
    jmethodID newIntent = (*env)->GetMethodID(env, intentClass, "<init>", "(Landroid/content/Context;Ljava/lang/Class;)V");
    if (newIntent == NULL) {
        LogD("C: ERROR - Failed to get Intent constructor");
        goto cleanup_intent;
    }

    intent = (*env)->NewObject(env, intentClass, newIntent, activity, activityClass);
    if (intent == NULL) {
        LogD("C: ERROR - Failed to create Intent");
        goto cleanup_intent;
    }

    LogD("C: Intent created successfully");

    // Устанавливаем флаги для перезапуска
    jmethodID setFlags = (*env)->GetMethodID(env, intentClass, "setFlags", "(I)Landroid/content/Intent;");
    if (setFlags != NULL) {
        (*env)->CallObjectMethod(env, intent, setFlags, 0x20000000 | 0x10000000 | 0x04000000); // SINGLE_TOP | NEW_TASK | CLEAR_TOP
        LogD("C: Flags set: SINGLE_TOP | NEW_TASK | CLEAR_TOP");
        caseException(env, "setFlags");
    }

    // Запускаем активность
    jmethodID startActivity = (*env)->GetMethodID(env, activityClass, "startActivity", "(Landroid/content/Intent;)V");
    if (startActivity != NULL) {
        LogD("C: Starting activity");
        (*env)->CallVoidMethod(env, activity, startActivity, intent);
        caseException(env, "startActivity");
        LogD("C: Activity started successfully");
    } else {
        LogD("C: ERROR - Failed to get startActivity method");
    }

    // Освобождение ресурсов
    if (intent) {
        (*env)->DeleteLocalRef(env, intent);
    }

cleanup_intent:
    if (intentClass) {
        (*env)->DeleteLocalRef(env, intentClass);
    }

cleanup_activity:
    if (activityClass) {
        (*env)->DeleteLocalRef(env, activityClass);
    }
}
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"

	"fyne.io/fyne/v2/driver"
	log "github.com/schollz/logger"
)

//export receiveURIFromIntent
func receiveURIFromIntent(uri *C.char) {
	if uri != nil {
		select {
		case uriFromIntent <- C.GoString(uri):
		default:
			log.Error("URI channel full, intent dropped")
		}
		C.free(unsafe.Pointer(uri))
	}
}

//export receiveTextFromIntent
func receiveTextFromIntent(text *C.char) {
	if text != nil {
		select {
		case textFromIntent <- C.GoString(text):
		default:
			log.Error("Text channel full, intent dropped")
		}
		C.free(unsafe.Pointer(text))
	}
}

func processIntent() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		log.Debug("Calling C.processIntent")

		C.processIntent(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.processIntent completed")
		return nil
	})
}

func setResult(ok bool) {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		var resultCode C.jint
		if ok {
			resultCode = -1 // RESULT_OK
		} else {
			resultCode = 0 // RESULT_CANCELED
		}

		log.Debugf("Calling C.setResult with code: %d", resultCode)

		C.setResult(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
			resultCode,
		)

		log.Debug("C.setResult completed")
		return nil
	})
}

// Это не закрывает приложение a и не закрывает окно w
// Просто открепляет окно w от активности  org.golang.app.GoNativeActivity
func finish() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		log.Debug("Calling C.finish")

		C.finish(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.finish completed")
		return nil
	})
}

// excludeFromRecents исключает приложение из списка недавних
func excludeFromRecents() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		log.Debug("Calling C.excludeFromRecents")

		C.excludeFromRecents(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.excludeFromRecents completed")
		return nil
	})
}

// Это просто связывает окно w
// c активностью org.golang.app.GoNativeActivity
func startActivity() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		log.Debug("Calling C.startActivitySimple")

		// Используем упрощенную версию
		C.startActivitySimple(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.startActivitySimple completed")
		return nil
	})
}

func start() {
	// excludeFromRecents()
	finish()
	sendNotification(nil, "Croc", AppClosed)
	time.Sleep(300 * time.Millisecond)
	startActivity()
}

func aContextString() string {
	var result string

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		if ac == nil {
			return nil
		}

		// Все три значения в одном формате
		acPtr := uintptr(unsafe.Pointer(ac))
		result = fmt.Sprintf("ac:%d Env:%d Ctx:%d", acPtr, ac.Env, ac.Ctx)

		return nil
	})

	return result
}

func ctxHandle() string {
	var handle string

	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)

		if ac == nil {
			return nil
		}

		handle = fmt.Sprintf("%d", ac.Ctx)
		return nil
	})

	return handle
}

func clearIntent() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		log.Debug("Calling C.clearIntent")

		C.clearIntent(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		log.Debug("C.clearIntent completed")
		return nil
	})
}
