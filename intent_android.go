//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

extern void receiveURIFromIntent(char* uri);
extern void receiveTextFromIntent(char* text);
extern void LogD(const char* message);

// Функция для установки результата активности
static void setResult(JNIEnv* env, jobject activity, jint resultCode) {
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for setResult");
        return;
    }

    jmethodID setResult = (*env)->GetMethodID(env, activity_class, "setResult", "(I)V");
    if (setResult == NULL) {
        LogD("C: ERROR - Failed to get setResult method");
        (*env)->DeleteLocalRef(env, activity_class);
        return;
    }

    char resultLog[64];
    snprintf(resultLog, sizeof(resultLog), "C: Setting result: %d", resultCode);
    LogD(resultLog);

    (*env)->CallVoidMethod(env, activity, setResult, resultCode);
    (*env)->DeleteLocalRef(env, activity_class);
}

// Функция для завершения активности
static void finish(JNIEnv* env, jobject activity) {
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for finish");
        return;
    }

    jmethodID finish = (*env)->GetMethodID(env, activity_class, "finish", "()V");
    if (finish == NULL) {
        LogD("C: ERROR - Failed to get finish method");
        (*env)->DeleteLocalRef(env, activity_class);
        return;
    }

    LogD("C: Finishing activity");
    (*env)->CallVoidMethod(env, activity, finish);
    (*env)->DeleteLocalRef(env, activity_class);
}

// Функция для исключения активности из недавних приложений и завершения
static void excludeFromRecents(JNIEnv* env, jobject activity) {
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class for excludeFromRecents");
        return;
    }

    // Получаем метод finishAndRemoveTask (доступен с API 21)
    jmethodID finishAndRemoveTask = (*env)->GetMethodID(env, activity_class, "finishAndRemoveTask", "()V");
    if (finishAndRemoveTask != NULL) {
        LogD("C: Using finishAndRemoveTask to exclude from recents");
        (*env)->CallVoidMethod(env, activity, finishAndRemoveTask);
    } else {
        LogD("C: finishAndRemoveTask not available, using finish()");

        // Альтернативный способ: устанавливаем флаг исключения из недавних
        jmethodID finish = (*env)->GetMethodID(env, activity_class, "finish", "()V");
        if (finish != NULL) {
            (*env)->CallVoidMethod(env, activity, finish);
        } else {
            LogD("C: ERROR - finish method also not available");
        }
    }

    (*env)->DeleteLocalRef(env, activity_class);
}

static void processIntent(JNIEnv* env, jobject activity) {
    LogD("C: === processIntent STARTED ===");

    // Получаем класс активности
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    if (activity_class == NULL) {
        LogD("C: ERROR - Failed to get activity class");
        return;
    }
    LogD("C: Activity class obtained successfully");

    // Получаем метод getIntent
    jmethodID get_intent = (*env)->GetMethodID(env, activity_class, "getIntent", "()Landroid/content/Intent;");
    if (get_intent == NULL) {
        LogD("C: ERROR - Failed to get getIntent method");
        (*env)->DeleteLocalRef(env, activity_class);
        return;
    }
    LogD("C: getIntent method obtained");

    // Вызываем getIntent()
    jobject intent = (*env)->CallObjectMethod(env, activity, get_intent);
    if (intent == NULL) {
        LogD("C: ERROR - Intent is NULL");
        (*env)->DeleteLocalRef(env, activity_class);
        return;
    }
    LogD("C: Intent obtained successfully");

    // Получаем класс Intent
    jclass intent_class = (*env)->GetObjectClass(env, intent);
    if (intent_class == NULL) {
        LogD("C: ERROR - Failed to get intent class");
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, intent);
        return;
    }
    LogD("C: Intent class obtained");

    // Получаем Action интента
    jmethodID getAction = (*env)->GetMethodID(env, intent_class, "getAction", "()Ljava/lang/String;");
    if (getAction == NULL) {
        LogD("C: ERROR - Failed to get getAction method");
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, intent);
        return;
    }

    jstring action = (*env)->CallObjectMethod(env, intent, getAction);
    const char *actionStr = NULL;
    int isSend = 0;
    int isView = 0;
    int isSendMultiple = 0;
    int isMain = 0;
    int shouldFinish = 0; // Флаг: нужно ли завершать активность
    int hasValidData = 0; // Флаг для отслеживания успешной обработки

    if (action != NULL) {
        actionStr = (*env)->GetStringUTFChars(env, action, NULL);
        if (actionStr != NULL) {
            char actionLog[256];
            snprintf(actionLog, sizeof(actionLog), "C: Intent action: %s", actionStr);
            LogD(actionLog);

            isSend = strcmp(actionStr, "android.intent.action.SEND") == 0;
            isView = strcmp(actionStr, "android.intent.action.VIEW") == 0;
            isSendMultiple = strcmp(actionStr, "android.intent.action.SEND_MULTIPLE") == 0;
            isMain = strcmp(actionStr, "android.intent.action.MAIN") == 0;

            (*env)->ReleaseStringUTFChars(env, action, actionStr);
        }
        (*env)->DeleteLocalRef(env, action);
    } else {
        LogD("C: Intent action is NULL");
    }

    // Для ACTION_MAIN - не завершаем активность, это запуск с иконки
    if (isMain) {
        LogD("C: MAIN intent - starting main app (not finishing activity)");
        (*env)->DeleteLocalRef(env, activity_class);
        (*env)->DeleteLocalRef(env, intent_class);
        (*env)->DeleteLocalRef(env, intent);
        LogD("C: === processIntent COMPLETED for MAIN ===");
        return;
    }

    // Для всех остальных интентов (SEND, VIEW, SEND_MULTIPLE) - завершаем активность после обработки
    // shouldFinish = 1;

    // First check ClipData
    LogD("C: Checking for ClipData...");
    jmethodID getClipData = (*env)->GetMethodID(env, intent_class, "getClipData", "()Landroid/content/ClipData;");
    if (getClipData != NULL) {
        jobject clipData = (*env)->CallObjectMethod(env, intent, getClipData);

        if (clipData != NULL) {
            LogD("C: ClipData found! Processing...");

            jclass clipData_class = (*env)->GetObjectClass(env, clipData);
            jmethodID getItemCount = (*env)->GetMethodID(env, clipData_class, "getItemCount", "()I");
            jmethodID getItemAt = (*env)->GetMethodID(env, clipData_class, "getItemAt", "(I)Landroid/content/ClipData$Item;");

            if (getItemCount != NULL && getItemAt != NULL) {
                jint itemCount = (*env)->CallIntMethod(env, clipData, getItemCount);
                char countLog[64];
                snprintf(countLog, sizeof(countLog), "C: ClipData item count: %d", itemCount);
                LogD(countLog);

                for (int i = 0; i < itemCount; i++) {
                    char itemLog[32];
                    snprintf(itemLog, sizeof(itemLog), "C: Processing item %d", i);
                    LogD(itemLog);

                    jobject item = (*env)->CallObjectMethod(env, clipData, getItemAt, i);
                    if (item != NULL) {
                        jclass item_class = (*env)->GetObjectClass(env, item);

                        // Process URI
                        jmethodID getUri = (*env)->GetMethodID(env, item_class, "getUri", "()Landroid/net/Uri;");
                        if (getUri != NULL) {
                            jobject uri = (*env)->CallObjectMethod(env, item, getUri);
                            if (uri != NULL) {
                                LogD("C: Found URI in ClipData item");
                                jclass uri_class = (*env)->GetObjectClass(env, uri);
                                jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                                if (to_string != NULL) {
                                    jstring uri_string = (*env)->CallObjectMethod(env, uri, to_string);
                                    if (uri_string != NULL) {
                                        const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                                        if (utf_str != NULL) {
                                            char uriLog[512];
                                            snprintf(uriLog, sizeof(uriLog), "C: Sending URI to Go: %s", utf_str);
                                            LogD(uriLog);
                                            receiveURIFromIntent(strdup(utf_str));
                                            hasValidData = 1;
                                            (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                                        }
                                        (*env)->DeleteLocalRef(env, uri_string);
                                    }
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
                                jclass text_class = (*env)->GetObjectClass(env, text);
                                jmethodID toString = (*env)->GetMethodID(env, text_class, "toString", "()Ljava/lang/String;");
                                if (toString != NULL) {
                                    jstring text_string = (*env)->CallObjectMethod(env, text, toString);
                                    if (text_string != NULL) {
                                        const char *text_str = (*env)->GetStringUTFChars(env, text_string, NULL);
                                        if (text_str != NULL) {
                                            char textLog[512];
                                            if (strlen(text_str) > 100) {
                                                char truncated[104];
                                                strncpy(truncated, text_str, 100);
                                                strcpy(truncated + 100, "...");
                                                snprintf(textLog, sizeof(textLog), "C: Sending text to Go (truncated): %s", truncated);
                                            } else {
                                                snprintf(textLog, sizeof(textLog), "C: Sending text to Go: %s", text_str);
                                            }
                                            LogD(textLog);
                                            receiveTextFromIntent(strdup(text_str));
                                            hasValidData = 1;
                                            (*env)->ReleaseStringUTFChars(env, text_string, text_str);
                                        }
                                        (*env)->DeleteLocalRef(env, text_string);
                                    }
                                }
                            }
                        }
                        (*env)->DeleteLocalRef(env, item);
                    }
                }
            }
            (*env)->DeleteLocalRef(env, clipData);

            if (hasValidData) {
                setResult(env, activity, 0); // RESULT_OK
                LogD("C: ClipData processing complete - setting RESULT_OK");
            } else {
                setResult(env, activity, -1); // RESULT_CANCELED
                LogD("C: ClipData processing complete - no valid data, setting RESULT_CANCELED");
            }

            // Завершаем активность только если это не MAIN интент
            if (shouldFinish) {
                finish(env, activity);
                LogD("C: Finishing activity after ClipData processing");
            }

            (*env)->DeleteLocalRef(env, activity_class);
            (*env)->DeleteLocalRef(env, intent_class);
            (*env)->DeleteLocalRef(env, intent);
            return;
        } else {
            LogD("C: No ClipData found");
        }
    } else {
        LogD("C: getClipData method not available");
    }

    // Затем проверяем ACTION_SEND text/plain
    if (isSend) {
        LogD("C: Checking for SEND text/plain...");
        jmethodID getType = (*env)->GetMethodID(env, intent_class, "getType", "()Ljava/lang/String;");
        if (getType != NULL) {
            jstring type = (*env)->CallObjectMethod(env, intent, getType);
            if (type != NULL) {
                const char *typeStr = (*env)->GetStringUTFChars(env, type, NULL);
                if (typeStr != NULL) {
                    char typeLog[128];
                    snprintf(typeLog, sizeof(typeLog), "C: Intent type: %s", typeStr);
                    LogD(typeLog);

                    int isTextPlain = strcmp(typeStr, "text/plain") == 0;
                    (*env)->ReleaseStringUTFChars(env, type, typeStr);

                    if (isTextPlain) {
                        LogD("C: Processing text/plain SEND intent");
                        jmethodID getStringExtra = (*env)->GetMethodID(env, intent_class,
                            "getStringExtra", "(Ljava/lang/String;)Ljava/lang/String;");
                        if (getStringExtra != NULL) {
                            jstring extraKey = (*env)->NewStringUTF(env, "android.intent.extra.TEXT");
                            jstring text = (*env)->CallObjectMethod(env, intent, getStringExtra, extraKey);
                            (*env)->DeleteLocalRef(env, extraKey);

                            if (text != NULL) {
                                const char *textStr = (*env)->GetStringUTFChars(env, text, NULL);
                                if (textStr != NULL) {
                                    char textLog[512];
                                    if (strlen(textStr) > 100) {
                                        char truncated[104];
                                        strncpy(truncated, textStr, 100);
                                        strcpy(truncated + 100, "...");
                                        snprintf(textLog, sizeof(textLog), "C: Sending SEND text to Go: %s", truncated);
                                    } else {
                                        snprintf(textLog, sizeof(textLog), "C: Sending SEND text to Go: %s", textStr);
                                    }
                                    LogD(textLog);
                                    receiveTextFromIntent(strdup(textStr));
                                    setResult(env, activity, 0); // RESULT_OK
                                    (*env)->ReleaseStringUTFChars(env, text, textStr);
                                }
                                (*env)->DeleteLocalRef(env, text);
                                LogD("C: SEND text processing complete - setting RESULT_OK");

                                // Завершаем активность только если это не MAIN интент
                                if (shouldFinish) {
                                    finish(env, activity);
                                    LogD("C: Finishing activity after SEND text processing");
                                }

                                (*env)->DeleteLocalRef(env, type);
                                (*env)->DeleteLocalRef(env, activity_class);
                                (*env)->DeleteLocalRef(env, intent_class);
                                (*env)->DeleteLocalRef(env, intent);
                                return;
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
        jmethodID get_data = (*env)->GetMethodID(env, intent_class, "getData", "()Landroid/net/Uri;");
        if (get_data != NULL) {
            jobject uri = (*env)->CallObjectMethod(env, intent, get_data);
            if (uri != NULL) {
                LogD("C: Found URI in VIEW intent");
                jclass uri_class = (*env)->GetObjectClass(env, uri);
                jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                if (to_string != NULL) {
                    jstring uri_string = (*env)->CallObjectMethod(env, uri, to_string);
                    if (uri_string != NULL) {
                        const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                        if (utf_str != NULL) {
                            char uriLog[512];
                            snprintf(uriLog, sizeof(uriLog), "C: Sending VIEW URI to Go: %s", utf_str);
                            LogD(uriLog);
                            receiveURIFromIntent(strdup(utf_str));
                            setResult(env, activity, 0); // RESULT_OK
                            (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                        }
                        (*env)->DeleteLocalRef(env, uri_string);
                    }
                }
                LogD("C: VIEW URI processing complete - setting RESULT_OK");

                // Завершаем активность только если это не MAIN интент
                if (shouldFinish) {
                    finish(env, activity);
                    LogD("C: Finishing activity after VIEW processing");
                }

                (*env)->DeleteLocalRef(env, uri);
                (*env)->DeleteLocalRef(env, activity_class);
                (*env)->DeleteLocalRef(env, intent_class);
                (*env)->DeleteLocalRef(env, intent);
                return;
            }
        }
    }

    // Handle ACTION_SEND (single content)
    if (isSend) {
        LogD("C: Checking for SEND stream...");
        jmethodID get_extra = (*env)->GetMethodID(env, intent_class, "getParcelableExtra", "(Ljava/lang/String;)Landroid/os/Parcelable;");
        if (get_extra != NULL) {
            jstring extra_key = (*env)->NewStringUTF(env, "android.intent.extra.STREAM");
            jobject send_uri = (*env)->CallObjectMethod(env, intent, get_extra, extra_key);
            (*env)->DeleteLocalRef(env, extra_key);

            if (send_uri != NULL) {
                LogD("C: Found stream URI in SEND intent");
                jclass uri_class = (*env)->GetObjectClass(env, send_uri);
                jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                if (to_string != NULL) {
                    jstring uri_string = (*env)->CallObjectMethod(env, send_uri, to_string);
                    if (uri_string != NULL) {
                        const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                        if (utf_str != NULL) {
                            char uriLog[512];
                            snprintf(uriLog, sizeof(uriLog), "C: Sending SEND stream URI to Go: %s", utf_str);
                            LogD(uriLog);
                            receiveURIFromIntent(strdup(utf_str));
                            setResult(env, activity, 0); // RESULT_OK
                            (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                        }
                        (*env)->DeleteLocalRef(env, uri_string);
                    }
                }
                LogD("C: SEND stream processing complete - setting RESULT_OK");

                // Завершаем активность только если это не MAIN интент
                if (shouldFinish) {
                    finish(env, activity);
                    LogD("C: Finishing activity after SEND stream processing");
                }

                (*env)->DeleteLocalRef(env, send_uri);
                (*env)->DeleteLocalRef(env, activity_class);
                (*env)->DeleteLocalRef(env, intent_class);
                (*env)->DeleteLocalRef(env, intent);
                return;
            }
        }
    }

    // Handle ACTION_SEND_MULTIPLE (multiple content)
    if (isSendMultiple) {
        LogD("C: Checking for SEND_MULTIPLE...");
        jmethodID get_array = (*env)->GetMethodID(env, intent_class, "getParcelableArrayListExtra", "(Ljava/lang/String;)Ljava/util/ArrayList;");
        if (get_array != NULL) {
            jstring array_key = (*env)->NewStringUTF(env, "android.intent.extra.STREAM");
            jobject uri_list = (*env)->CallObjectMethod(env, intent, get_array, array_key);
            (*env)->DeleteLocalRef(env, array_key);

            if (uri_list != NULL) {
                LogD("C: Found URI list in SEND_MULTIPLE intent");
                jclass array_list_class = (*env)->GetObjectClass(env, uri_list);
                jmethodID get_size = (*env)->GetMethodID(env, array_list_class, "size", "()I");
                jmethodID get_item = (*env)->GetMethodID(env, array_list_class, "get", "(I)Ljava/lang/Object;");

                if (get_size != NULL && get_item != NULL) {
                    jint size = (*env)->CallIntMethod(env, uri_list, get_size);
                    char sizeLog[64];
                    snprintf(sizeLog, sizeof(sizeLog), "C: SEND_MULTIPLE URI count: %d", size);
                    LogD(sizeLog);

                    for (int i = 0; i < size; i++) {
                        char itemLog[32];
                        snprintf(itemLog, sizeof(itemLog), "C: Processing URI %d", i);
                        LogD(itemLog);

                        jobject current_uri = (*env)->CallObjectMethod(env, uri_list, get_item, i);
                        if (current_uri != NULL) {
                            jclass uri_class = (*env)->GetObjectClass(env, current_uri);
                            jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                            if (to_string != NULL) {
                                jstring uri_string = (*env)->CallObjectMethod(env, current_uri, to_string);
                                if (uri_string != NULL) {
                                    const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                                    if (utf_str != NULL) {
                                        char uriLog[512];
                                        snprintf(uriLog, sizeof(uriLog), "C: Sending SEND_MULTIPLE URI to Go: %s", utf_str);
                                        LogD(uriLog);
                                        receiveURIFromIntent(strdup(utf_str));
                                        hasValidData = 1;
                                        (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
                                    }
                                    (*env)->DeleteLocalRef(env, uri_string);
                                }
                            }
                            (*env)->DeleteLocalRef(env, current_uri);
                        }
                    }

                    if (hasValidData) {
                        setResult(env, activity, 0); // RESULT_OK
                        LogD("C: SEND_MULTIPLE processing complete - setting RESULT_OK");
                    } else {
                        setResult(env, activity, -1); // RESULT_CANCELED
                        LogD("C: SEND_MULTIPLE processing complete - no valid data, setting RESULT_CANCELED");
                    }

                    // Завершаем активность только если это не MAIN интент
                    if (shouldFinish) {
                        finish(env, activity);
                        LogD("C: Finishing activity after SEND_MULTIPLE processing");
                    }
                }
                (*env)->DeleteLocalRef(env, uri_list);
                (*env)->DeleteLocalRef(env, activity_class);
                (*env)->DeleteLocalRef(env, intent_class);
                (*env)->DeleteLocalRef(env, intent);
                return;
            }
        }
    }

    // Если дошли сюда - данные не найдены
    LogD("C: No matching intent data found");
    setResult(env, activity, -1); // RESULT_CANCELED

    // Завершаем активность только если это не MAIN интент
    if (shouldFinish) {
        finish(env, activity);
        LogD("C: Finishing activity after no data found");
    }

    (*env)->DeleteLocalRef(env, activity_class);
    (*env)->DeleteLocalRef(env, intent_class);
    (*env)->DeleteLocalRef(env, intent);
    LogD("C: === processIntent COMPLETED ===");
}
*/
import "C"
import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

//export receiveURIFromIntent
func receiveURIFromIntent(uri *C.char) {
	if uri != nil {
		goString := C.GoString(uri)
		LogD("Go: Received URI from intent: " + goString)
		select {
		case uriFromIntent <- goString:
			LogD("Go: URI sent to channel successfully")
		default:
			LogD("Go: WARNING - URI channel full, intent dropped")
		}
		C.free(unsafe.Pointer(uri))
	}
}

//export receiveTextFromIntent
func receiveTextFromIntent(text *C.char) {
	if text != nil {
		goString := C.GoString(text)
		logText := goString
		if len(logText) > 100 {
			logText = logText[:100] + "..."
		}
		LogD("Go: Received text from intent: " + logText)
		select {
		case textFromIntent <- goString:
			LogD("Go: Text sent to channel successfully")
		default:
			LogD("Go: WARNING - Text channel full, intent dropped")
		}
		C.free(unsafe.Pointer(text))
	}
}

func processIntent() {
	LogD("Go: setupIntentHandler called")

	driver.RunNative(func(ctx interface{}) error {
		LogD("Go: driver.RunNative started")

		ac := ctx.(*driver.AndroidContext)
		LogD("Go: Calling C.processIntent")

		C.processIntent(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		LogD("Go: C.processIntent completed")
		return nil
	})
}

func finish() {
	LogD("Go: finish called")

	driver.RunNative(func(ctx interface{}) error {
		LogD("Go: driver.RunNative started")

		ac := ctx.(*driver.AndroidContext)
		LogD("Go: Calling C.finish")

		C.finish(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		LogD("Go: C.finish completed")
		return nil
	})
}

// excludeFromRecents завершает приложение и исключает его из списка недавних приложений
func excludeFromRecents() {
	LogD("Go: excludeFromRecents called")

	driver.RunNative(func(ctx interface{}) error {
		LogD("Go: driver.RunNative started for excludeFromRecents")

		ac := ctx.(*driver.AndroidContext)
		LogD("Go: Calling C.excludeFromRecents")

		C.excludeFromRecents(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)

		LogD("Go: C.excludeFromRecents completed")
		return nil
	})
}
