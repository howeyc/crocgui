# План: Протокол статистики для анализа адаптивного гистерезиса

## Цель

Добавить в adaptive catch-up loop и handleIncomingChunk **только логирование** параметров:
- размер текущего чанка
- behind
- liveJitter
- playbackRate
- текущие SLOMO/BUST/EMERGENCY

Никаких изменений в логику SLOMO/BUST/EMERGENCY — сначала собираем статистику,
потом на основе данных придумаем эвристику.

## Принцип

- **Размер чанка** на приёмнике уже отражает и площадь кадра, и битрейт, и ключевые кадры
- **behind** — главное состояние adaptive loop
- **Не усредняем** — логируем каждый тик, чтобы видеть скачки от ключевых кадров
- **Не нужен передатчик** — всё измеряется локально на приёмнике

---

## Изменения в videocall.html

### 1. Новые переменные для статистики (рядом с liveJitter, строка ~599)

```javascript
var lastChunkSize = 0;              // размер последнего полученного чанка (байт)
var _lastHystLogTime = 0;           // троттлинг логов (мс)
var HYST_LOG_INTERVAL_MS = 2000;    // интервал логирования
```

### 2. В handleIncomingChunk (строка ~2295): запомнить размер чанка

После `diagRecvBytes += data.byteLength;` добавить:

```javascript
lastChunkSize = data.byteLength;
```

### 3. В adaptive catch-up setInterval (строка ~1351): логирование

Внутри блока `if (remoteVideo.buffered && remoteVideo.buffered.length > 0)`,
после вычисления `behind` и до трёхзонного rate control, добавить:

```javascript
// Протокол статистики для анализа гистерезиса
var now = Date.now();
if (now - _lastHystLogTime >= HYST_LOG_INTERVAL_MS) {
    diagLog('HYST chunk=' + lastChunkSize +
        ' behind=' + behind.toFixed(3) +
        ' jit=' + liveJitter.toFixed(3) +
        ' rate=' + remoteVideo.playbackRate.toFixed(2) +
        ' SLOMO=' + SLOMO.toFixed(2) +
        ' BUST=' + BUST.toFixed(2) +
        ' EMER=' + EMERGENCY_BEHIND.toFixed(2) +
        ' buf=[' + bufStart.toFixed(1) + '-' + bufEnd.toFixed(1) + ']' +
        ' ct=' + ct.toFixed(1));
    _lastHystLogTime = now;
}
```

### 4. Сброс в restartMSE (строка ~2439)

Добавить:

```javascript
lastChunkSize = 0;
_lastHystLogTime = 0;
```

---

## Сводка

| Что | Где | Суть |
|-----|-----|------|
| `lastChunkSize` | переменная ~599 | размер последнего чанка |
| `_lastHystLogTime` | переменная ~599 | троттлинг логов |
| `lastChunkSize = data.byteLength` | handleIncomingChunk ~2295 | запомнить размер |
| `diagLog HYST ...` | adaptive loop ~1360 | лог каждые 2 сек |
| Сброс переменных | restartMSE ~2439 | чистка при пересоздании MSE |

**Никаких изменений в SLOMO/BUST/EMERGENCY_BEHIND.**
**Никаких новых полей в sendSettings.**
**Всё только на приёмнике.**
