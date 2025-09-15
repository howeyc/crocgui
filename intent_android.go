//go:build android

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

extern void receiveURIFromIntent(char* uri);
extern void receiveTextFromIntent(char* text);

static void processIntent(JNIEnv* env, jobject activity) {
    jclass activity_class = (*env)->GetObjectClass(env, activity);
    jmethodID get_intent = (*env)->GetMethodID(env, activity_class, "getIntent", "()Landroid/content/Intent;");
    jobject intent = (*env)->CallObjectMethod(env, activity, get_intent);

    if (intent == NULL) return;

    jclass intent_class = (*env)->GetObjectClass(env, intent);
    if (intent_class == NULL) return;

    // Get action
    jmethodID getAction = (*env)->GetMethodID(env, intent_class, "getAction", "()Ljava/lang/String;");
    jstring action = (*env)->CallObjectMethod(env, intent, getAction);

    const char *actionStr = NULL;
    int isSend = 0;
    int isView = 0;
    int isSendMultiple = 0;

    if (action != NULL) {
        actionStr = (*env)->GetStringUTFChars(env, action, NULL);
        isSend = strcmp(actionStr, "android.intent.action.SEND") == 0;
        isView = strcmp(actionStr, "android.intent.action.VIEW") == 0;
        isSendMultiple = strcmp(actionStr, "android.intent.action.SEND_MULTIPLE") == 0;
        (*env)->ReleaseStringUTFChars(env, action, actionStr);
        (*env)->DeleteLocalRef(env, action);
    }

    // First check ClipData - это ОЧЕНЬ ВАЖНО для Android 9 и ниже
    jmethodID getClipData = (*env)->GetMethodID(env, intent_class, "getClipData", "()Landroid/content/ClipData;");
    jobject clipData = (*env)->CallObjectMethod(env, intent, getClipData);

    if (clipData != NULL) {
        jclass clipData_class = (*env)->GetObjectClass(env, clipData);
        jmethodID getItemCount = (*env)->GetMethodID(env, clipData_class, "getItemCount", "()I");
        jint itemCount = (*env)->CallIntMethod(env, clipData, getItemCount);

        jmethodID getItemAt = (*env)->GetMethodID(env, clipData_class, "getItemAt", "(I)Landroid/content/ClipData$Item;");

        for (int i = 0; i < itemCount; i++) {
            jobject item = (*env)->CallObjectMethod(env, clipData, getItemAt, i);
            if (item != NULL) {
                jclass item_class = (*env)->GetObjectClass(env, item);
                jmethodID getUri = (*env)->GetMethodID(env, item_class, "getUri", "()Landroid/net/Uri;");
                jobject uri = (*env)->CallObjectMethod(env, item, getUri);

                if (uri != NULL) {
                    jclass uri_class = (*env)->GetObjectClass(env, uri);
                    jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                    jstring uri_string = (*env)->CallObjectMethod(env, uri, to_string);

                    const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                    receiveURIFromIntent(strdup(utf_str));
                    (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);

                    (*env)->DeleteLocalRef(env, uri_string);
                    (*env)->DeleteLocalRef(env, uri);
                }

                // Also check for text
                jmethodID getText = (*env)->GetMethodID(env, item_class, "getText", "()Ljava/lang/CharSequence;");
                jobject text = (*env)->CallObjectMethod(env, item, getText);

                if (text != NULL) {
                    jclass text_class = (*env)->GetObjectClass(env, text);
                    jmethodID toString = (*env)->GetMethodID(env, text_class, "toString", "()Ljava/lang/String;");
                    jstring text_string = (*env)->CallObjectMethod(env, text, toString);

                    const char *text_str = (*env)->GetStringUTFChars(env, text_string, NULL);
                    receiveTextFromIntent(strdup(text_str));
                    (*env)->ReleaseStringUTFChars(env, text_string, text_str);

                    (*env)->DeleteLocalRef(env, text_string);
                }

                (*env)->DeleteLocalRef(env, item);
            }
        }
        (*env)->DeleteLocalRef(env, clipData);
        return; // Если нашли ClipData, выходим - это приоритетно
    }

    // Затем проверяем ACTION_SEND text/plain
    if (isSend) {
        jmethodID getType = (*env)->GetMethodID(env, intent_class, "getType", "()Ljava/lang/String;");
        jstring type = (*env)->CallObjectMethod(env, intent, getType);

        if (type != NULL) {
            const char *typeStr = (*env)->GetStringUTFChars(env, type, NULL);
            int isTextPlain = strcmp(typeStr, "text/plain") == 0;
            (*env)->ReleaseStringUTFChars(env, type, typeStr);
            (*env)->DeleteLocalRef(env, type);

            if (isTextPlain) {
                jmethodID getStringExtra = (*env)->GetMethodID(env, intent_class,
                    "getStringExtra", "(Ljava/lang/String;)Ljava/lang/String;");
                jstring extraKey = (*env)->NewStringUTF(env, "android.intent.extra.TEXT");
                jstring text = (*env)->CallObjectMethod(env, intent, getStringExtra, extraKey);
                (*env)->DeleteLocalRef(env, extraKey);

                if (text != NULL) {
                    const char *textStr = (*env)->GetStringUTFChars(env, text, NULL);
                    receiveTextFromIntent(strdup(textStr));
                    (*env)->ReleaseStringUTFChars(env, text, textStr);
                    (*env)->DeleteLocalRef(env, text);
                    return;
                }
            }
        }
    }

    // Handle ACTION_VIEW (single URI)
    if (isView) {
        jmethodID get_data = (*env)->GetMethodID(env, intent_class, "getData", "()Landroid/net/Uri;");
        jobject uri = (*env)->CallObjectMethod(env, intent, get_data);

        if (uri != NULL) {
            jclass uri_class = (*env)->GetObjectClass(env, uri);
            jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
            jstring uri_string = (*env)->CallObjectMethod(env, uri, to_string);

            const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
            receiveURIFromIntent(strdup(utf_str));
            (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
            return;
        }
    }

    // Handle ACTION_SEND (single content)
    if (isSend) {
        jmethodID get_extra = (*env)->GetMethodID(env, intent_class, "getParcelableExtra", "(Ljava/lang/String;)Landroid/os/Parcelable;");
        jstring extra_key = (*env)->NewStringUTF(env, "android.intent.extra.STREAM");
        jobject send_uri = (*env)->CallObjectMethod(env, intent, get_extra, extra_key);
        (*env)->DeleteLocalRef(env, extra_key);

        if (send_uri != NULL) {
            jclass uri_class = (*env)->GetObjectClass(env, send_uri);
            jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
            jstring uri_string = (*env)->CallObjectMethod(env, send_uri, to_string);

            const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
            receiveURIFromIntent(strdup(utf_str));
            (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);
            return;
        }
    }

    // Handle ACTION_SEND_MULTIPLE (multiple content)
    if (isSendMultiple) {
        jmethodID get_array = (*env)->GetMethodID(env, intent_class, "getParcelableArrayListExtra", "(Ljava/lang/String;)Ljava/util/ArrayList;");
        jstring array_key = (*env)->NewStringUTF(env, "android.intent.extra.STREAM");
        jobject uri_list = (*env)->CallObjectMethod(env, intent, get_array, array_key);
        (*env)->DeleteLocalRef(env, array_key);

        if (uri_list != NULL) {
            jclass array_list_class = (*env)->GetObjectClass(env, uri_list);
            jmethodID get_size = (*env)->GetMethodID(env, array_list_class, "size", "()I");
            jmethodID get_item = (*env)->GetMethodID(env, array_list_class, "get", "(I)Ljava/lang/Object;");
            jint size = (*env)->CallIntMethod(env, uri_list, get_size);

            for (int i = 0; i < size; i++) {
                jobject current_uri = (*env)->CallObjectMethod(env, uri_list, get_item, i);
                jclass uri_class = (*env)->GetObjectClass(env, current_uri);
                jmethodID to_string = (*env)->GetMethodID(env, uri_class, "toString", "()Ljava/lang/String;");
                jstring uri_string = (*env)->CallObjectMethod(env, current_uri, to_string);

                const char *utf_str = (*env)->GetStringUTFChars(env, uri_string, NULL);
                receiveURIFromIntent(strdup(utf_str));
                (*env)->ReleaseStringUTFChars(env, uri_string, utf_str);

                (*env)->DeleteLocalRef(env, current_uri);
                (*env)->DeleteLocalRef(env, uri_string);
            }
            return;
        }
    }
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
		uriFromIntent <- C.GoString(uri)
		C.free(unsafe.Pointer(uri))
	}
}

//export receiveTextFromIntent
func receiveTextFromIntent(text *C.char) {
	if text != nil {
		textFromIntent <- C.GoString(text)
		C.free(unsafe.Pointer(text))
	}
}

func setupIntentHandler() {
	driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		C.processIntent(
			(*C.JNIEnv)(unsafe.Pointer(ac.Env)),
			(C.jobject)(unsafe.Pointer(ac.Ctx)),
		)
		return nil
	})
}
