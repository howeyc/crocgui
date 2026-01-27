//go:build android

// read_android.go

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <android/log.h>
#include <string.h>

#define LogD(...) __android_log_print(ANDROID_LOG_DEBUG, "croc", __VA_ARGS__)

// Структура для потокового чтения
typedef struct {
    JNIEnv* env;
    jobject inputStream;
    jclass inputStreamClass;
    jmethodID readMethod;
    jmethodID closeMethod;
    jbyteArray jBuffer;
    jbyte* buffer;
    jint bufferSize;
} StreamState;

// Проверка исключений
static jboolean checkException(JNIEnv* env, const char* msg) {
    jthrowable exception = (*env)->ExceptionOccurred(env);
    if (exception) {
        LogD("Exception in %s", msg);
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
        return JNI_TRUE;
    }
    return JNI_FALSE;
}

static jboolean CheckException(JNIEnv* env, const char* msg) {
    return checkException(env, msg);
}

// Открытие потока для чтения - упрощенная версия
static jlong openDocumentStream(JNIEnv* env, jobject activity, const char* uriStr) {
    jobject contentResolver = NULL;
    jobject uri = NULL;
    jobject inputStream = NULL;
    jclass activityClass = NULL;
    jclass uriClass = NULL;
    jclass resolverClass = NULL;
    jstring juriStr = NULL;

    LogD("openDocumentStream: Starting for URI: %s", uriStr);

    StreamState* state = malloc(sizeof(StreamState));
    if (!state) {
        LogD("openDocumentStream: Failed to allocate state structure");
        return 0;
    }
    memset(state, 0, sizeof(StreamState));

    // Получаем ContentResolver
    LogD("openDocumentStream: Getting ContentResolver");
    activityClass = (*env)->GetObjectClass(env, activity);
    if (!activityClass) {
        LogD("openDocumentStream: Failed to get activity class");
        goto error;
    }

    jmethodID getContentResolver = (*env)->GetMethodID(env, activityClass, "getContentResolver", "()Landroid/content/ContentResolver;");
    if (!getContentResolver) {
        LogD("openDocumentStream: Failed to get getContentResolver method");
        goto error;
    }

    contentResolver = (*env)->CallObjectMethod(env, activity, getContentResolver);
    if (checkException(env, "getContentResolver")) {
        LogD("openDocumentStream: Exception getting ContentResolver");
        goto error;
    }
    if (!contentResolver) {
        LogD("openDocumentStream: ContentResolver is NULL");
        goto error;
    }

    // Парсим URI
    LogD("openDocumentStream: Parsing URI");
    uriClass = (*env)->FindClass(env, "android/net/Uri");
    if (!uriClass) {
        LogD("openDocumentStream: Failed to find Uri class");
        goto error;
    }

    jmethodID parseMethod = (*env)->GetStaticMethodID(env, uriClass, "parse", "(Ljava/lang/String;)Landroid/net/Uri;");
    if (!parseMethod) {
        LogD("openDocumentStream: Failed to get parse method");
        goto error;
    }

    juriStr = (*env)->NewStringUTF(env, uriStr);
    if (!juriStr) {
        LogD("openDocumentStream: Failed to create Java string from URI");
        goto error;
    }

    uri = (*env)->CallStaticObjectMethod(env, uriClass, parseMethod, juriStr);
    if (checkException(env, "parse URI")) {
        LogD("openDocumentStream: Exception parsing URI");
        goto error;
    }
    if (!uri) {
        LogD("openDocumentStream: Parsed URI is NULL");
        goto error;
    }

    // Пробуем открыть поток
    LogD("openDocumentStream: Opening input stream");
    resolverClass = (*env)->GetObjectClass(env, contentResolver);
    if (!resolverClass) {
        LogD("openDocumentStream: Failed to get resolver class");
        goto error;
    }

    jmethodID openInputStreamMethod = (*env)->GetMethodID(env, resolverClass, "openInputStream", "(Landroid/net/Uri;)Ljava/io/InputStream;");
    if (!openInputStreamMethod) {
        LogD("openDocumentStream: openInputStream method not found");
        goto error;
    }

    inputStream = (*env)->CallObjectMethod(env, contentResolver, openInputStreamMethod, uri);

    if (checkException(env, "openInputStream")) {
        LogD("openDocumentStream: openInputStream exception");
        goto error;
    }

    if (!inputStream) {
        LogD("openDocumentStream: openInputStream returned NULL");
        goto error;
    }

    LogD("openDocumentStream: Successfully opened input stream");

    // Настраиваем состояние
    state->env = env;
    state->inputStream = (*env)->NewGlobalRef(env, inputStream);
    if (!state->inputStream) {
        LogD("openDocumentStream: Failed to create global ref for input stream");
        goto error;
    }

    state->inputStreamClass = (*env)->FindClass(env, "java/io/InputStream");
    if (!state->inputStreamClass) {
        LogD("openDocumentStream: Failed to find InputStream class");
        goto error;
    }
    state->inputStreamClass = (*env)->NewGlobalRef(env, state->inputStreamClass);

    state->readMethod = (*env)->GetMethodID(env, state->inputStreamClass, "read", "([B)I");
    if (!state->readMethod) {
        LogD("openDocumentStream: Failed to get read method");
        goto error;
    }

    state->closeMethod = (*env)->GetMethodID(env, state->inputStreamClass, "close", "()V");
    if (!state->closeMethod) {
        LogD("openDocumentStream: Failed to get close method");
        goto error;
    }

    state->bufferSize = 64 * 1024; // 64KB
    state->jBuffer = (*env)->NewByteArray(env, state->bufferSize);
    if (!state->jBuffer) {
        LogD("openDocumentStream: Failed to create byte array");
        goto error;
    }
    state->jBuffer = (*env)->NewGlobalRef(env, state->jBuffer);

    state->buffer = (*env)->GetByteArrayElements(env, state->jBuffer, NULL);
    if (!state->buffer) {
        LogD("openDocumentStream: Failed to get byte array elements");
        goto error;
    }

    // Освобождаем локальные ссылки
    if (juriStr) (*env)->DeleteLocalRef(env, juriStr);
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (activityClass) (*env)->DeleteLocalRef(env, activityClass);
    if (contentResolver) (*env)->DeleteLocalRef(env, contentResolver);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (resolverClass) (*env)->DeleteLocalRef(env, resolverClass);
    if (inputStream) (*env)->DeleteLocalRef(env, inputStream);

    LogD("openDocumentStream: Stream setup completed successfully");
    return (jlong)(intptr_t)state;

error:
    LogD("openDocumentStream: Failed to open stream, cleaning up");
    if (state) {
        if (state->buffer) {
            (*env)->ReleaseByteArrayElements(env, state->jBuffer, state->buffer, JNI_ABORT);
        }
        if (state->jBuffer) {
            (*env)->DeleteGlobalRef(env, state->jBuffer);
        }
        if (state->inputStreamClass) {
            (*env)->DeleteGlobalRef(env, state->inputStreamClass);
        }
        if (state->inputStream) {
            (*env)->DeleteGlobalRef(env, state->inputStream);
        }
        free(state);
    }
    if (juriStr) (*env)->DeleteLocalRef(env, juriStr);
    if (uri) (*env)->DeleteLocalRef(env, uri);
    if (activityClass) (*env)->DeleteLocalRef(env, activityClass);
    if (contentResolver) (*env)->DeleteLocalRef(env, contentResolver);
    if (uriClass) (*env)->DeleteLocalRef(env, uriClass);
    if (resolverClass) (*env)->DeleteLocalRef(env, resolverClass);
    if (inputStream) (*env)->DeleteLocalRef(env, inputStream);

    return 0;
}

// Чтение данных из потока
static jint readDocumentStream(JNIEnv* env, jlong streamPtr, jbyteArray goBuffer, jint length) {
    StreamState* state = (StreamState*)(intptr_t)streamPtr;
    if (!state || !state->inputStream) {
        LogD("readDocumentStream: Stream is closed or invalid");
        return -1;
    }

    // LogD("readDocumentStream: Starting read, requested length: %d", length);

    // Читаем данные в Java буфер
    jint bytesRead = (*env)->CallIntMethod(env, state->inputStream, state->readMethod, state->jBuffer);
    // LogD("readDocumentStream: Java read method returned: %d", bytesRead);

    if (checkException(env, "read from stream")) {
        LogD("readDocumentStream: Read exception occurred");
        return -1;
    }

    // Обрабатываем возвращаемые значения правильно
    if (bytesRead == -1) {
        LogD("readDocumentStream: EOF reached - normal end of file");
        return 0;  // Возвращаем 0 для EOF, а не -1
    } else if (bytesRead == 0) {
        LogD("readDocumentStream: Read returned 0 bytes (может быть нормально для некоторых потоков)");
        return 0;
    } else if (bytesRead > 0) {
        // LogD("readDocumentStream: Read successful, copying %d bytes to Go buffer", bytesRead);

        // ВАЖНО: Копируем данные из state->buffer в goBuffer
        (*env)->SetByteArrayRegion(env, goBuffer, 0, bytesRead, state->buffer);

        // Проверяем исключение после копирования
        if (checkException(env, "copy to Go buffer")) {
            LogD("readDocumentStream: Exception while copying to Go buffer");
            return -1;
        }
        // LogD("readDocumentStream: Successfully copied %d bytes", bytesRead);
        return bytesRead;
    } else {
        LogD("readDocumentStream: Read returned unexpected value: %d", bytesRead);
        return -1;
    }
}

// Закрытие потока
static void closeDocumentStream(JNIEnv* env, jlong streamPtr) {
    StreamState* state = (StreamState*)(intptr_t)streamPtr;
    if (!state) {
        LogD("closeDocumentStream: Stream state is NULL");
        return;
    }

    LogD("closeDocumentStream: Starting stream closure");

    // Закрываем InputStream
    if (state->inputStream && state->closeMethod) {
        LogD("closeDocumentStream: Calling Java close method");
        (*env)->CallVoidMethod(env, state->inputStream, state->closeMethod);

        if (checkException(env, "close stream")) {
            LogD("closeDocumentStream: Exception during Java close method");
        } else {
            LogD("closeDocumentStream: Java close method completed successfully");
        }
    } else {
        LogD("closeDocumentStream: InputStream or closeMethod is NULL");
    }

    // Освобождаем ресурсы
    LogD("closeDocumentStream: Releasing resources");

    if (state->buffer) {
        (*env)->ReleaseByteArrayElements(env, state->jBuffer, state->buffer, JNI_ABORT);
        LogD("closeDocumentStream: Released byte array elements");
    }

    if (state->jBuffer) {
        (*env)->DeleteGlobalRef(env, state->jBuffer);
        LogD("closeDocumentStream: Deleted global ref for jBuffer");
    }

    if (state->inputStreamClass) {
        (*env)->DeleteGlobalRef(env, state->inputStreamClass);
        LogD("closeDocumentStream: Deleted global ref for inputStreamClass");
    }

    if (state->inputStream) {
        (*env)->DeleteGlobalRef(env, state->inputStream);
        LogD("closeDocumentStream: Deleted global ref for inputStream");
    }

    free(state);
    LogD("closeDocumentStream: Stream closure completed");
}

// Вспомогательные функции
static jbyteArray NewByteArray(JNIEnv* env, jint length) {
    return (*env)->NewByteArray(env, length);
}

static void SetByteArrayRegion(JNIEnv* env, jbyteArray array, jint start, jint len, jbyte* buf) {
    (*env)->SetByteArrayRegion(env, array, start, len, buf);
}

static void DeleteLocalRef(JNIEnv* env, jobject obj) {
    (*env)->DeleteLocalRef(env, obj);
}

static jboolean IsNull(JNIEnv* env, jobject obj) {
    return obj == NULL;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unsafe"

	log "github.com/schollz/logger"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/storage"
)

// BufferedDocumentReader для потокового чтения больших файлов
type BufferedDocumentReader struct {
	uri    fyne.URI
	stream C.jlong // Указатель на нативный поток
	closed bool
	buffer []byte // Внутренний буфер
	bufPos int    // Текущая позиция в буфере
	bufLen int    // Количество данных в буфере
}

// reader создает потоковый ридер для больших файлов
func reader(uri fyne.URI) (fyne.URIReadCloser, error) {
	var streamPtr C.jlong

	// log.Debugf("reader: Attempting to open document stream for: %s", uri.String())

	err := driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
		activity := C.jobject(unsafe.Pointer(ac.Ctx))

		uriStr := C.CString(uri.String())
		defer C.free(unsafe.Pointer(uriStr))

		// log.Debug("reader: Calling native openDocumentStream")
		streamPtr = C.openDocumentStream(env, activity, uriStr)
		if streamPtr == 0 {
			// log.Debug("reader: Native openDocumentStream returned 0")
			return errors.New("failed to open document stream")
		}

		// log.Debug("reader: Native openDocumentStream succeeded")
		return nil
	})

	if err != nil {
		log.Debugf("reader: Error opening document stream: %v", err)
		return nil, err
	}

	// log.Debug("reader: Successfully created BufferedDocumentReader")
	return &BufferedDocumentReader{
		uri:    uri,
		stream: streamPtr,
		closed: false,
		buffer: make([]byte, 64*1024), // 64KB буфер
		bufPos: 0,
		bufLen: 0,
	}, nil
}

// fillBuffer заполняет внутренний буфер данными из нативного потока
func (b *BufferedDocumentReader) fillBuffer() error {
	var bytesRead C.jint

	err := driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))

		// Создаем Java массив, который будет передан в нативную функцию
		jBuffer := C.NewByteArray(env, C.jint(len(b.buffer)))
		if C.IsNull(env, C.jobject(unsafe.Pointer(jBuffer))) != 0 {
			return errors.New("failed to create Java byte array")
		}
		defer C.DeleteLocalRef(env, C.jobject(unsafe.Pointer(jBuffer)))

		bytesRead = C.readDocumentStream(env, b.stream, jBuffer, C.jint(len(b.buffer)))

		if bytesRead < 0 {
			return errors.New("error reading from document stream")
		} else if bytesRead == 0 {
			b.bufLen = 0
			b.bufPos = 0
			return nil
		} else if bytesRead > 0 {
			// Копируем данные из jBuffer в b.buffer
			tempBuffer := make([]byte, bytesRead)
			C.SetByteArrayRegion(env, jBuffer, 0, bytesRead, (*C.jbyte)(unsafe.Pointer(&tempBuffer[0])))

			if C.CheckException(env, C.CString("SetByteArrayRegion")) != 0 {
				return errors.New("error copying data from Java to Go")
			}

			copy(b.buffer, tempBuffer)
			b.bufLen = int(bytesRead)
			b.bufPos = 0
		}

		return nil
	})

	return err
}

// Read читает данные из документа
func (b *BufferedDocumentReader) Read(p []byte) (n int, err error) {
	if b.closed {
		// log.Debug("Read: Reader is closed")
		return 0, errors.New("reader is closed")
	}

	if len(p) == 0 {
		log.Debug("Read: Zero-length buffer provided")
		return 0, nil
	}

	// log.Debugf("Read: Requested %d bytes, current bufPos: %d, bufLen: %d", len(p), b.bufPos, b.bufLen)

	// Если в буфере нет данных, читаем новную порцию
	if b.bufPos >= b.bufLen {
		// log.Debug("Read: Buffer empty, calling fillBuffer")
		err = b.fillBuffer()
		if err != nil {
			// log.Debugf("Read: fillBuffer error: %v", err)
			return 0, err
		}

		// Если после заполнения буфера все еще нет данных - EOF
		if b.bufLen == 0 {
			// log.Debug("Read: EOF reached")
			return 0, io.EOF
		}
	}

	// Копируем данные из буфера в предоставленный слайс
	copyLen := b.bufLen - b.bufPos
	if copyLen > len(p) {
		copyLen = len(p)
	}

	// log.Debugf("Read: Copying %d bytes from internal buffer", copyLen)
	copy(p[:copyLen], b.buffer[b.bufPos:b.bufPos+copyLen])
	b.bufPos += copyLen

	// log.Debugf("Read: Successfully read %d bytes, new bufPos: %d", copyLen, b.bufPos)
	return copyLen, nil
}

// Close закрывает поток и освобождает ресурсы
func (b *BufferedDocumentReader) Close() error {
	if b.closed {
		log.Debug("Close: Reader already closed")
		return errors.New("already closed")
	}

	b.closed = true
	log.Debug("Close: Starting stream closure")

	var closeError error
	err := driver.RunNative(func(ctx interface{}) error {
		ac := ctx.(*driver.AndroidContext)
		env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))

		log.Debug("Close: Calling native closeDocumentStream")
		C.closeDocumentStream(env, b.stream)

		// Проверяем исключения после закрытия
		if C.CheckException(env, C.CString("closeDocumentStream")) != 0 {
			log.Debug("Close: Exception during native stream closure")
			closeError = errors.New("exception during stream closure")
		} else {
			log.Debug("Close: Native stream closed successfully")
		}

		b.stream = 0
		return nil
	})

	if err != nil {
		log.Debugf("Close: Error running native function: %v", err)
		return err
	}

	if closeError != nil {
		log.Debugf("Close: Stream closure completed with error: %v", closeError)
		return closeError
	}

	log.Debug("Close: Stream closed successfully")
	return nil
}

// URI возвращает URI документа
func (b *BufferedDocumentReader) URI() fyne.URI {
	return b.uri
}

func Reader(u fyne.URI) (r fyne.URIReadCloser, err error) {
	if u == nil {
		err = fmt.Errorf("uri is nul")
		return
	}
	// if u.Scheme() == "content" {
	// 	return reader(u)
	// }
	if !canRead(u) {
		err = fmt.Errorf("uri not readable")
		return
	}
	return storage.Reader(u)
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
		log.Errorf("canRead: %v", err)
		return false
	}
	if !ok {
		return false
	}
	if false {
		log.Debug("CanRead %s", uri)

		r, err := storage.Reader(uri)
		if err != nil {
			log.Errorf("reader: %v", err)
			return false
		}
		defer r.Close()

		p := make([]byte, 1)
		_, err = r.Read(p)
		if err != nil {
			log.Errorf("read: %v", err)
			return false
		}
	}
	return true
}
