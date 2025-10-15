#include <android/log.h>
#include <jni.h>
#include <stdlib.h>
#include <string.h>

void logToAndroid(const char* tag, const char* message);
jboolean checkIsOreoOrLater(JNIEnv* env);
void createCrocNotificationChannel(JNIEnv* env, jobject context);
void showCrocNotification(JNIEnv* env, jobject context, char* title, char* content);