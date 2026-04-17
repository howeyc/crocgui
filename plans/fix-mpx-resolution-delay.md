# План исправления задержки разрешения при смене mpx

## Хронология из логов

### SETMPX 0.05→0.5 — что происходит

```mermaid
sequenceDiagram
    participant H as Host браузер
    participant W as WebSocket
    participant G as Guest браузер
    participant C as Guest камера
    participant R as Guest MediaRecorder

    H->>H: setMpx 0.5
    H->>W: settings mpx=0.5
    W->>G: settings mpx=0.5
    G->>G: handlePeerSettings
    G->>C: applyDesiredResolution 1550x322 БЕЗ await
    Note over C: applyConstraints начал работу асинхронно
    G->>R: restartMediaRecorder СРАЗУ
    Note over R: REC codec res=102x490 СТАРОЕ!
    R-->>W: init segment 102x490
    W-->>H: chunks со старым разрешением
    C-->>G: applyConstraints завершён 1550x322
    Note over G: RES applied peer desire 1550x322
    Note over R: Камера теперь 1550x322 но рекордер уже работает
    R-->>W: новый init segment 1550x322
    W-->>H: MSE RESTART - двойной рестарт!
```

### SCREEN toggle off→on — что происходит

```mermaid
sequenceDiagram
    participant H as Host браузер
    participant W as WebSocket
    participant G as Guest браузер
    participant C as Guest камера
    participant R as Guest MediaRecorder

    H->>H: SCREEN toggle wantVideo=false
    Note over H: remoteVideo.style.visibility = hidden
    H->>W: settings wantVideo=false
    W->>G: settings wantVideo=false
    G->>G: handlePeerSettings
    G->>C: applyDesiredResolution БЕЗ await
    G->>R: restartMediaRecorder comp=true
    Note over R: REC codec res=322x1550 СТАРОЕ!
    C-->>G: applyConstraints завершён 1550x322

    H->>H: SCREEN toggle wantVideo=true
    Note over H: remoteVideo.style.visibility = visible
    Note over H: БРАУЗЕР ПЕРЕРИСОВЫВАЕТ видеоэлемент!
    H->>W: settings wantVideo=true
    W->>G: settings wantVideo=true
    G->>R: restartMediaRecorder comp=true
    Note over R: К этому моменту камера УЖЕ 1550x322
    Note over R: Но REC codec res=322x1550 - ЕЩЁ СТАРОЕ в логах!
    C-->>G: applyConstraints завершён 1550x322
```

## Две проблемы из логов

### Проблема 1: Рекордер стартует со старым разрешением

applyDesiredResolution - async с await applyConstraints внутри.
Вызывается БЕЗ await в handlePeerSettings строка ~1467.
Рекордер стартует ДО завершения applyConstraints.

Доказательство из логов - ВСЕГДА REC codec res=СТАРОЕ перед RES applied:

| Сценарий       | REC codec res | RES applied   | Флаг рестарта |
|----------------|---------------|---------------|---------------|
| SETMPX 0.05→0.5 | 102x490      | 1550x322      | res=true      |
| SCREEN off     | 322x1550      | 1550x322      | comp=true     |
| SCREEN on      | 322x1550      | 1550x322      | comp=true     |
| SETMPX 0.5→0.05 | 322x1550    | 490x102       | res=true      |

### Проблема 2: Браузер не перерисовывает видеоэлемент

При SCREEN toggle applySettings меняет remoteVideo.style.visibility:
- wantVideo=false → visibility=hidden
- wantVideo=true → visibility=visible

Это принудительно перерисовывает элемент → новое разрешение видно.

При SETMPX visibility НЕ меняется → браузер не обновляет отображение.

## План исправления

### Шаг 1: Скрыть/показать remoteVideo в setMpx — РЕАЛИЗОВАНО

videocall.html setMpx строка 497:
```javascript
remoteVideo.style.visibility = 'hidden';
setTimeout(function() {
    if (S.wantVideo) remoteVideo.style.visibility = 'visible';
}, 1000);
```

### Шаг 2: await applyDesiredResolution в handlePeerSettings

Чтобы рекордер стартовал с ПРАВИЛЬНЫМ разрешением:

1. Сделать handlePeerSettings async:
```javascript
async function handlePeerSettings(msg, initial) {
```

2. Добавить await перед applyDesiredResolution строка ~1467:
```javascript
await applyDesiredResolution(best.w, best.h);
```

3. Добавить .catch на вызов в WS onmessage строка ~1614:
```javascript
handlePeerSettings(msg, initial).catch(function(e) {
    diagLog('handlePeerSettings error: ' + e.message);
});
```

4. Перенести lastHandledMpx обновление до await чтобы избежать гонок:
```javascript
if (Math.abs(msg.mpx - lastHandledMpx) > 0.001) {
    resolutionChanged = true;
    lastHandledMpx = msg.mpx;  // ← перенести сюда
}
```

### Ожидаемый результат после Шага 2

```mermaid
sequenceDiagram
    participant H as Host браузер
    participant W as WebSocket
    participant G as Guest браузер
    participant C as Guest камера
    participant R as Guest MediaRecorder

    H->>H: setMpx 0.5
    H->>W: settings mpx=0.5
    W->>G: settings mpx=0.5
    G->>G: handlePeerSettings ASYNC
    G->>C: AWAIT applyDesiredResolution 1550x322
    C-->>G: applyConstraints завершён 1550x322
    G->>R: restartMediaRecorder ПОСЛЕ переключения
    Note over R: REC codec res=1550x322 ПРАВИЛЬНОЕ!
    R-->>W: init segment 1550x322
    W-->>H: chunks с правильным разрешением
    Note over H: Один MSE restart - сразу показывает новое разрешение
```
