# План: Логика приватности кодеков в videocall.html

## Требование
Когда один участник (Пир A) отключает все аудио кодеки (Opus + AAC) или все видео кодеки (VP8 + AVC), у его собеседника (Пир B) автоматически отключается соответствующее устройство (Mic или Cam).

**Приватность:** Обратное включение кодеков у Пира A НЕ должно автоматически включать Mic/Cam у Пира B. Пир B должен сам нажать свою кнопку Mic/Cam.

## Логика работы

```mermaid
sequenceDiagram
    participant A as Пир A
    participant B as Пир B

    Note over A: Отключает Opus и AAC
    A->>B: settings: opus=false, aac=false
    Note over B: handlePeerSettings видит opus=false AND aac=false
    Note over B: S.audio = false, освобождает аудио треки

    Note over A: Включает Opus обратно
    A->>B: settings: opus=true, aac=false
    Note over B: Mic остаётся выключенным - приватность!
    Note over B: Пир B должен сам нажать Mic

    Note over A: Отключает VP8 и AVC
    A->>B: settings: vp8=false, h264=false
    Note over B: handlePeerSettings видит vp8=false AND h264=false
    Note over B: S.video = false, освобождает видео треки

    Note over A: Включает VP8 обратно
    A->>B: settings: vp8=true, h264=false
    Note over B: Cam остаётся выключенной - приватность!
    Note over B: Пир B должен сам нажать Cam
```

## Изменения в коде

### Единственное изменение: функция handlePeerSettings() - строка ~1806

Вставить блок проверки кодеков пира ПОСЛЕ строки `peerSettings = msg;` (строка 1808) и ПОСЛЕ проверки `if (!isActive || !localStream) return;` (строка 1810), но ДО проверки `if (initial) return;` (строка 1854):

```javascript
// === PRIVACY: если у пира все аудио кодеки выключены — отключаем свой микрофон ===
var peerNoAudio = msg.opus === false && msg.aac === false;
if (peerNoAudio && S.audio) {
    diagLog('PRIVACY: peer disabled all audio codecs, turning off my Mic');
    S.audio = false;
    if (localStream) {
        localStream.getAudioTracks().forEach(function(t) { t.stop(); localStream.removeTrack(t); });
    }
    applySettings(); saveSettings();
    if (!S.audio && !S.video) {
        if (mediaRecorder && mediaRecorder.state !== 'inactive') {
            mediaRecorder.stop(); recorderStarted = false;
        }
    } else if (recorderStarted) {
        restartMediaRecorder('settings');
    }
}

// === PRIVACY: если у пира все видео кодеки выключены — отключаем свою камеру ===
var peerNoVideo = msg.vp8 === false && msg.h264 === false;
if (peerNoVideo && S.video) {
    diagLog('PRIVACY: peer disabled all video codecs, turning off my Cam');
    S.video = false;
    if (localStream) {
        localStream.getVideoTracks().forEach(function(t) { t.stop(); localStream.removeTrack(t); });
    }
    applySettings(); saveSettings();
    if (!S.audio && !S.video) {
        if (mediaRecorder && mediaRecorder.state !== 'inactive') {
            mediaRecorder.stop(); recorderStarted = false;
        }
    } else if (recorderStarted) {
        restartMediaRecorder('settings');
    }
}
```

### Ключевые моменты
- Проверяем `msg.opus === false && msg.aac === false` — строго false, не undefined
- Только ОДНОНАПРАВЛЕННОЕ действие: выключение. Включение кодеков у пира НЕ включает Mic/Cam
- Освобождаем треки (stop + removeTrack) — физически отпускаем устройство
- Обновляем UI через applySettings(), сохраняем через saveSettings()
- Рестарт рекордера при необходимости
