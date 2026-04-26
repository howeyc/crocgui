# Video Call: протокол обмена сообщениями и состояния

## Архитектура

```mermaid
graph TD
    subgraph Host - Browser
        H_UI[UI кнопки]
        H_S[Объект S - настройки]
        H_REC[MediaRecorder]
        H_MSE[MSE - проигрывание]
        H_ECHO[echo=true при старте]
    end
    subgraph Go Server
        WS[WebSocket relay]
        ECHO{Remote peer online?}
    end
    subgraph Guest - Browser
        G_UI[UI кнопки]
        G_S[Объект S - настройки]
        G_REC[MediaRecorder]
        G_MSE[MSE - проигрывание]
    end

    H_S -->|sendSettings role:host| WS
    H_REC -->|ArrayBuffer chunks| WS
    G_S -->|sendSettings role:guest| WS
    G_REC -->|ArrayBuffer chunks| WS

    WS --> ECHO
    ECHO -->|Peer online| G_S
    ECHO -->|Peer online| G_MSE
    ECHO -->|Peer online| H_S
    ECHO -->|Peer online| H_MSE
    ECHO -->|Peer offline - echo| H_S
    ECHO -->|Peer offline - echo| H_MSE
```

## Эхо-режим

Когда хост один в комнате — нет удалённого пира — сервер возвращает все сообщения и чанки обратно отправителю. Это позволяет хосту видеть своё локальное видео как удалённое.

**Серверная логика** — в `videocall.go`: если `room.getWS(remotePeer) == nil`, текстовые и бинарные сообщения пересылаются обратно отправителю.

**Клиентская логика:**

| Переменная | Тип | Описание |
|------------|-----|----------|
| `echo` | `bool` | Локальная переменная — НЕ входит в JSON. `true` у хоста при старте |
| `guestSettings` | `object/null` | Последние настройки от гостя. Сохраняются в `localStorage.vc_peerSettings` только при `role === guest` |

**Поведение хоста в `ws.onopen`:**
1. Установить `echo = true`
2. Загрузить `guestSettings` из `localStorage` → если есть, вызвать `handlePeerSettings(saved, true)`
3. Если нет — создать fallback из собственного `S` + `recordCodecs` + `playCodecs` → `handlePeerSettings(fallback, true)`
4. Запустить `setupMediaRecorder()` — рекордер работает в эхо-режиме

**Поведение хоста при получении `cmd:settings`:**
- `handlePeerSettings(msg, false)` вызывается **всегда** — и для эха, и для реальных настроек
- Если `echo === true` и `msg.role === 'guest'` → первый реальный гость! Установить `echo = false`, отправить `sendSettings()` однократно
- Если `echo === true` и `msg.role !== 'guest'` → эхо собственных настроек, `handlePeerSettings` всё равно вызывается (обновляет negotiated codec и т.д.)

**Поведение гостя в `ws.onopen`:**
1. Отправить `sendSettings()` — хост получит их и ответит своими настройками

## Инициализация — init flow

Оба пира проходят одинаковый путь: `init()` → `startCall()` → `connectWS()`. Никаких HTTP-запросов `waitForPeer`/`joinRoom` — всё через WS.

### Хост — один в комнате — эхо-режим

```mermaid
sequenceDiagram
    participant H as Host
    participant WS as Go Server

    H->>H: init - isCreator=true
    H->>H: startCall - connectWS
    H->>WS: WebSocket connect
    WS-->>H: onopen
    H->>H: echo = true
    H->>H: Загрузить guestSettings из localStorage
    alt Есть сохранённые guestSettings
        H->>H: handlePeerSettings - saved - initial=true
    else Нет сохранённых
        H->>H: fallbackSettings = копия S + recordCodecs + playCodecs
        H->>H: handlePeerSettings - fallback - initial=true
    end
    H->>H: setupMediaRecorder
    H->>WS: init segment - binary
    WS-->>H: echo — binary back
    H->>H: setupMSE - вижу своё видео
    Note over H: Хост видит своё локальное видео как удалённое
```

### Гость подключается — переход из эхо в нормальный режим

```mermaid
sequenceDiagram
    participant H as Host
    participant WS as Go Server
    participant G as Guest

    G->>G: init - isCreator=false
    G->>G: startCall - connectWS
    G->>WS: WebSocket connect
    WS-->>G: onopen
    G->>WS: cmd:settings role=guest
    WS->>H: cmd:settings role=guest - не эхо! удалённый пир онлайн
    H->>H: echo=true, role=guest → echo=false
    H->>H: handlePeerSettings - guest settings
    H->>H: restartMediaRecorder - с настройками гостя
    H->>WS: cmd:settings role=host
    WS->>G: cmd:settings role=host
    G->>G: handlePeerSettings - host settings
    G->>G: setupMediaRecorder
    Note over H,G: Двусторонняя связь установлена
```

## WS-сообщения

### 1. `cmd: settings` — настройки пира

**Направление:** Peer → Server → Remote Peer (или обратно отправителю в эхо-режиме)
**Когда отправляется:**
- Гость: при подключении к WS (`ws.onopen`)
- Хост: однократно в ответ на первые `settings` от гостя (`role=guest`)
- Оба: при нажатии любой кнопки управления
- Оба: при resize окна (через ResizeObserver, debounce 500ms)

**Поля:**

| Поле | Тип | Описание | Кнопка |
|------|-----|----------|--------|
| `role` | string | `host` или `guest` — идентифицирует отправителя | — |
| `audio` | bool | Мой микрофон вкл/выкл | `micBtn` 🎤 |
| `video` | bool | Моя камера вкл/выкл | `camBtn` 📷 |
| `wantAudio` | bool | Хочу аудио от пира | `remoteMuteBtn` 🔈 |
| `wantVideo` | bool | Хочу видео от пира | `remoteScreenBtn` 💻 |
| `vp8` | bool | VP8 кодек доступен | `vp8Btn` VP8 |
| `h264` | bool | H.264 кодек доступен | `h264Btn` AVC |
| `opus` | bool | Opus кодек доступен | `opusBtn` Opus |
| `aac` | bool | AAC кодек доступен | `aacBtn` AAC |
| `ec` | bool | Echo Cancellation | `ecBtn` EC |
| `ns` | bool | Noise Suppression | `nsBtn` NS |
| `agc` | bool | Auto Gain Control | `agcBtn` AGC |
| `mpx` | float | Мегапиксели для запроса пиру | `.5` `.2` `.1` `.05` |
| `recordCodecs` | array | Доступные кодеки для записи | — |
| `playCodecs` | array | Доступные кодеки для воспроизведения | — |
| `width` | int | Фактическая ширина видео | — |
| `height` | int | Фактическая высота видео | — |
| `frameRate` | float | FPS | — |
| `displayW` | int | Ширина MSE-контейнера | — |
| `displayH` | int | Высота MSE-контейнера | — |

### 2. `cmd: initCodec` — фактический кодек из init segment

**Направление:** Peer → Server → Remote Peer
**Когда отправляется:** На **каждый** init segment от MediaRecorder (TCP гарантирует порядок — до бинарного чанка)
**Поля:**

| Поле | Тип | Описание |
|------|-----|----------|
| `codec` | string | Полная MIME-строка, например `video/webm;codecs=vp8,opus` |

**Получатель:** **Единственная точка управления MSE.** Если MSE не создан → `setupMSE()`. Если кодек изменился → `restartMSE()` + `setupMSE()`. Если кодек тот же → ничего.

### 3. `peer_left` — отключение пира

**Направление:** Server → Peer  
**Когда отправляется:** При отключении пира от WS

### 4. ArrayBuffer chunks — медиаданные

**Направление:** Peer → Server → Remote Peer  
**Тип:** `websocket.BinaryMessage`  
**Содержимое:** WebM chunks от MediaRecorder, включая init segment

## Обработка settings — фильтрация эха и обработка

```mermaid
flowchart TD
    MSG[Получено cmd: settings] --> ECHO{echo=true AND role=guest?}
    ECHO -->|Да| DISABLE[echo=false + sendSettings однократно]
    ECHO -->|Нет — эхо или обычный режим| EXTRACT
    DISABLE --> EXTRACT[Извлечь displayW/displayH]
    EXTRACT --> DISPLAY{displayChanged?}
    DISPLAY -->|Да| SAVE_DISPLAY[Сохранить peerDisplayW/H]
    DISPLAY -->|Нет| NEGOTIATE
    SAVE_DISPLAY --> NEGOTIATE[Negotiate codec]
    NEGOTIATE --> MPX[applyDesiredResolution - mpx пира]
    MPX --> INITIAL{initial?}
    INITIAL -->|Да| RETURN[return - startCall запустит рекордер]
    INITIAL -->|Нет| CHECK_CODEC{codecChanged?}
    CHECK_CODEC -->|Да| RESTART[restartMediaRecorder]
    CHECK_CODEC -->|Нет| CHECK_COMP{compositionChanged?}
    CHECK_COMP -->|Да| RESTART
    CHECK_COMP -->|Нет| CHECK_STOP{wasStopped?}
    CHECK_STOP -->|Да| RESTART
    CHECK_STOP -->|Нет| CHECK_RES{resolutionChanged or displayChanged?}
    CHECK_RES -->|Да| ON_FLY[RES changed on the fly - no restart]
    CHECK_RES -->|Нет| NOTHING[Ничего не делаем]
```

## Условия рестарта MediaRecorder

| Условие | Значение | Рестарт? |
|---------|----------|----------|
| `codecChanged` | Кодек записи изменился | ✅ Да |
| `compositionChanged` | Состав A/V потоков изменился | ✅ Да |
| `wasStopped` | Рекордер не запущен | ✅ Да |
| `resolutionChanged` | mpx изменился | ❌ Нет — applyConstraints |
| `displayChanged` | Размер окна пира изменился | ❌ Нет — applyConstraints |

## Управление MSE — только через `cmd:initCodec`

MSE отслеживает `currentMseCodec` — фактический кодек, с которым создан sourceBuffer.

**`cmd:initCodec` handler — единственная точка управления MSE:**

| Состояние MSE | Кодек | Действие |
|---------------|-------|----------|
| Не создан | любой | `setupMSE()` с этим кодеком |
| Создан, `codec !== currentMseCodec` | другой | `restartMSE()` (teardown) + `setupMSE()` |
| Создан, `codec === currentMseCodec` | тот же | Ничего |

**`handleIncomingChunk()`** — только `mseQueue.push()` + `processMSEQueue()`. Никакого анализа чанков.

**`restartMSE()`** — только teardown (очистка очереди, уничтожение MSE). Не создаёт MSE.

**`startCall()`** — упрощён до `isActive = true; connectWS()`. Не создаёт MSE, не ждёт пира. MSE создаётся только из `cmd:initCodec` handler. Рекордер запускается в `ws.onopen` (хост) или в `handlePeerSettings()` (гость).

## Локальные настройки — объект S

```javascript
S = {
    audio: true,          // мой микрофон → micBtn
    video: true,          // моя камера → camBtn
    wantAudio: true,      // хочу аудио от пира → remoteMuteBtn
    wantVideo: true,      // хочу видео от пира → remoteScreenBtn
    vp8: true,            // VP8 кодек → vp8Btn
    h264: false,          // H.264 кодек → h264Btn
    opus: true,           // Opus кодек → opusBtn
    aac: false,           // AAC кодек → aacBtn
    ec: true,             // Echo Cancellation → ecBtn
    ns: true,             // Noise Suppression → nsBtn
    agc: true,            // Auto Gain Control → agcBtn
    mpx: 0.1,             // Мегапиксели → .5/.2/.1/.05 кнопки
    pipCorner: 0          // Позиция PiP — ТОЛЬКО локально, НЕ отправляется
}
```

## Кнопки и их влияние

### Sender — управление моими устройствами

| Кнопка | ID | Переключает | Локальный эффект | Отправляется пиру |
|--------|-----|-------------|------------------|-------------------|
| 🎤 Mic | `micBtn` | `S.audio` | release/acquire audio track | `audio` в settings |
| 📷 Cam | `camBtn` | `S.video` | release/acquire video track | `video` в settings |

### Receiver — управление что я хочу от пира

| Кнопка | ID | Переключает | Локальный эффект | Отправляется пиру |
|--------|-----|-------------|------------------|-------------------|
| 🔈 Audio | `remoteMuteBtn` | `S.wantAudio` | `remoteVideo.muted` | `wantAudio` в settings |
| 💻 Video | `remoteScreenBtn` | `S.wantVideo` | `remoteVideo.visibility` | `wantVideo` в settings |

### Кодеки — что поддерживает мой браузер

| Кнопка | ID | Переключает | Влияет на negotiation |
|--------|-----|-------------|----------------------|
| VP8 | `vp8Btn` | `S.vp8` | recordCodecs/playCodecs |
| AVC | `h264Btn` | `S.h264` | recordCodecs/playCodecs |
| Opus | `opusBtn` | `S.opus` | recordCodecs/playCodecs |
| AAC | `aacBtn` | `S.aac` | recordCodecs/playCodecs |

### Аудио фильтры

| Кнопка | ID | Переключает | Локальный эффект |
|--------|-----|-------------|------------------|
| EC | `ecBtn` | `S.ec` | `applyConstraints({echoCancellation})` |
| NS | `nsBtn` | `S.ns` | `applyConstraints({noiseSuppression})` |
| AGC | `agcBtn` | `S.agc` | `applyConstraints({autoGainControl})` |

### Разрешение — запрос пиру

| Кнопка | ID | Устанавливает | Пир применяет |
|--------|-----|---------------|---------------|
| .5 | `mpx12Btn` | `S.mpx = 0.5` | `applyDesiredResolution()` |
| .2 | `mpx15Btn` | `S.mpx = 0.2` | `applyDesiredResolution()` |
| .1 | `mpx110Btn` | `S.mpx = 0.1` | `applyDesiredResolution()` |
| .05 | `mpx120Btn` | `S.mpx = 0.05` | `applyDesiredResolution()` |

### Локальные кнопки — НЕ отправляются пиру

| Кнопка | ID | Действие |
|--------|-----|---------|
| 💡 Fit | `fitContainBtn` | Режим отображения remote video |
| ☐ Fullscreen | `fullscreenBtn` | Полноэкранный режим |

## Поток данных при смене разрешения

```mermaid
sequenceDiagram
    participant User
    participant PeerA as Peer A - Sender
    participant WS as Go Server
    participant PeerB as Peer B - Receiver

    User->>PeerA: Нажатие .5
    PeerA->>PeerA: S.mpx = 0.5
    PeerA->>WS: cmd:settings mpx=0.5
    WS->>PeerB: cmd:settings mpx=0.5
    PeerB->>PeerB: findBestResolution 0.5
    PeerB->>PeerB: applyDesiredResolution - applyConstraints
    Note over PeerB: Трек меняет разрешение на лету
    Note over PeerB: MediaRecorder НЕ перезапускается
    Note over PeerB: Init segment НЕ генерируется
    Note over PeerB: MSE НЕ перезапускается
```

## Поток данных при смене кодека

```mermaid
sequenceDiagram
    participant User
    participant PeerA as Peer A - Sender
    participant WS as Go Server
    participant PeerB as Peer B - Receiver

    User->>PeerA: Нажатие camBtn - выкл камеру
    PeerA->>PeerA: S.video = false
    PeerA->>PeerA: release video track
    PeerA->>WS: cmd:settings video=false
    WS->>PeerB: cmd:settings video=false
    PeerB->>PeerB: compositionChanged = true
    PeerB->>PeerB: restartMediaRecorder - audio-only
    PeerB->>WS: init segment - audio/webm;codecs=opus
    WS->>PeerA: init segment chunk
    PeerA->>PeerA: detectedCodec !== currentMseCodec
    PeerA->>PeerA: restartMSE - audio-only codec
```
