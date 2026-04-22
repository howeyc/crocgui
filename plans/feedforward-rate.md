# Feedforward Rate Control + Timestamp Protocol

## Проблема

Текущий feedback подход (`rate = A + K × behind`) реагирует на уже возникшее отставание.
При TCP burst/jitter буфер расходится быстрее → декодер теряет состояние → stall.
Логи показали `rs=1-2, bufAhead=0.7-0.9` — данные есть, но декодер не может возобновиться с P-frame.

## Решение: Feedforward + SafeRate

### 1. Timestamp на блобах

Отправитель добавляет 8 байт `Date.now()` (Float64 little-endian) перед каждым блобом:
```
[8 bytes Float64: Date.now()][blob data]
```
Go relay не парсит — просто ретранслирует.

### 2. Приёмник: все блобы в MSE, incomingRate по скользящему окну

```javascript
// handleIncomingChunk:
var sendTime = new DataView(data).getFloat64(0, true);
if (sendTime > prevSendTime) {  // пропуск не-по-порядку
    prevSendTime = sendTime;
    var now = performance.now();
    incomingRateWindow.push({wall: now, st: sendTime});
    // Удаляем записи старше 1 секунды wall-time
    while (incomingRateWindow.length > 1 && now - incomingRateWindow[0].wall > INCOMING_RATE_WINDOW_MS) {
        incomingRateWindow.shift();
    }
    if (incomingRateWindow.length >= 2) {
        var mediaSpan = last.st - first.st;   // ms отправителя
        var wallSpan = last.wall - first.wall; // ms наше wall time
        incomingRate = mediaSpan / wallSpan;
    }
}
// Всегда добавляем в MSE (сохраняем keyframes)
var payload = data.slice(8);
mseQueue.push(payload);
```

**Ключевые отличия от предыдущей версии:**
- Не используем абсолютное время (age) — только относительное (mediaSpan / wallSpan)
- Не считаем блобы — скользящее окно по wall-time (1 секунда)
- Пропускаем блобы с sendTime ≤ предыдущего (не-по-порядку)
- Все блобы добавляются в MSE (сохраняют keyframes)

### 3. Feedforward rate в setInterval

```javascript
// Секция 3: feedforward rate (нормальная работа)
var bufAhead = bufEnd - ct;
var safeRate = bufAhead / LIVE_TARGET_S;  // пропорциональный контроллер
var newRate = Math.max(MIN_RATE, Math.min(MAX_RATE, Math.min(incomingRate, safeRate)));
remoteVideo.playbackRate = newRate;
```

### 4. Константы

| Константа | Значение | Описание |
|-----------|----------|----------|
| `ADAPTIVE_INTERVAL_MS` | 100 | Интервал поллинга |
| `ADAPTIVE_INTERVAL_S` | 0.1 | Интервал в секундах |
| `INCOMING_RATE_WINDOW_MS` | 1000 | Окно измерения incomingRate (wall time) |
| `MIN_RATE` | 0.85 | Минимальный playbackRate (15% замедление, net growth 0.15с/с) |
| `MAX_RATE` | 1.25 | Максимальный playbackRate (25% ускорение, net drain 0.25с/с) |

### 5. Сценарии

#### Норма: 10 блобов/с
```
incomingRate = 1.0, bufAhead = 0.5
safeRate = 0.5 / 0.5 = 1.0
rate = min(1.0, 1.0) = 1.0 ✓ (равновесие)
```

#### Jitter: 9 блобов/с
```
incomingRate = 0.9, bufAhead = 0.3
safeRate = 0.3 / 0.5 = 0.6
rate = clamp(min(0.9, 0.6), 0.85, 1.25) = 0.85 ✓ (MIN_RATE, копим буфер)
```

#### TCP burst: 11 блобов/с
```
incomingRate = 1.1, bufAhead = 0.8
safeRate = 0.8 / 0.5 = 1.5
rate = clamp(min(1.1, 1.5), 0.85, 1.25) = 1.1 ✓ (ускоряемся, догоняем буфер)
```

#### У края буфера
```
incomingRate = 1.1, bufAhead = 0.25
safeRate = 0.25 / 0.5 = 0.5
rate = clamp(min(1.1, 0.5), 0.85, 1.25) = 0.85 ✓ (MIN_RATE, копим буфер)
```

## Что НЕ меняется

- Stall detection (ct-based + readyState)
- Emergency seek (seek на keyframe если в буфере, иначе точка равновесия + rate=1.0)
- Gap detection
- didSeekInTick защита
- Диагноз в логе [keyframe/underrun/gap]

## Keyframe detection по размеру блоба

Keyframe (I-frame) блобы в 5-10× больше обычных P-frame (VP8). VP8 генерирует keyframes каждые ~16-20 кадров (~1.6-2с при 10fps). Это позволяет детектировать их по размеру без парсинга видеопотока.

**Проблема ложных срабатываний:** при высоком разрешении P-frames имеют вариацию до 2× из-за движения. Порог 1.5× вызывал каскад ложных срабатываний (крупный P-frame → «keyframe» → исключается из EMA → avgBlobSize не растёт → следующий P-frame тоже > 1.5× → каскад). Решение: порог 1.5× + минимальный интервал 10 кадров (`MIN_KEYFRAME_INTERVAL_S`). При низком разрешении (364×272) VP8 keyframes ~1.8× от среднего — порог 2.0× их пропускал. MIN_KEYFRAME_INTERVAL_S блокирует каскад даже при пороге 1.5×.

### Алгоритм

1. **Фильтр мелких блобов:** `chunkSize > 500` — init segment (1 байт) исключается из EMA и keyframe detection
2. **EMA обычных блобов:** `avgBlobSize = avgBlobSize * 0.9 + chunkSize * 0.1` — обновляется только для обычных блобов (keyframe исключаются)
3. **Детектирование:** `chunkSize > avgBlobSize * KEYFRAME_SIZE_RATIO (1.5)` → предполагаемый keyframe
4. **Интервал:** `timeSinceLastKf >= MIN_KEYFRAME_INTERVAL_S (10/TARGET_FPS)` — блокирует каскад ложных срабатываний
5. **Media time:** `pendingKeyframeMediaTime = bufEnd` до append; подтверждается в `onMSEUpdateEnd` → `lastKeyframeMediaTime`
6. **Защита в trimBuffer:** Не обрезать последний keyframe — `removeEnd = min(removeEnd, lastKeyframeMediaTime)`

### Stall handler: две стратегии

| stallReason | lastKeyframeMediaTime | Действие |
|-------------|----------------------|----------|
| `keyframe` | `>= bufStart && >= ct` (в буфере и не позади ct) | `currentTime = lastKeyframeMediaTime - TARGET_FPS/200` — seek между кадрами, браузер найдёт ближайший keyframe |
| `keyframe` | `< bufStart` или `< ct` (вне буфера или позади ct) | `currentTime = max(bufEnd - LIVE_TARGET_S*2, ct + 0.2)` — точка равновесия |
| `underrun`/`gap` | — | `currentTime = max(bufEnd - LIVE_TARGET_S*2, ct + 0.2)` |

**Почему `kf >= ct`, а не `kf > ct`:** после stall seek на `kf - 0.05`, ct ≈ kf, и строгое `kf > ct` ложно → emergency seek прыгает на equilibrium вместо keyframe, пропуская секунды видео.

**keyframe позади ct:** если `lastKeyframeMediaTime < ct`, seek к нему бессмысленен — видео уже прошло этот keyframe. Fall through к точке равновесия. Лог содержит `kf_age` (ct - lastKeyframeMediaTime) для диагностики.

**Почему seek на `kf - 0.05` (между кадрами):** seek точно на keyframe может попасть на предыдущий кадр из-за погрешности `pendingKeyframeMediaTime`. Смещение -0.05 (полкадра при 10fps) ставит ct между кадрами — браузер найдёт ближайший keyframe и начнёт декодирование с него.

### Сброс при MSE restart

`avgBlobSize = 0`, `pendingKeyframeMediaTime = -1`, `lastKeyframeMediaTime = -1`

## Почему rate=1.0 при stall, а MAX_RATE=1.5 при feedforward

Ускорение (rate > 1.0) сразу после stall вызывает потерю keyframe — декодер не успевает обработать I-frame. Поэтому stall recovery использует hardcoded rate=1.0.

В нормальном режиме (feedforward) MAX_RATE=1.5 позволяет плавно догонять буфер при TCP burst (incomingRate > 1.0), не вызывая stall.

## Реализация (выполнено)

1. ✅ Вернуть полную stall-обработку (убрать [keyframe?] ветку)
2. ✅ Изменить ADAPTIVE_INTERVAL_MS на 100
3. ✅ Добавить timestamp в sendChunk() — 8 байт Float64 little-endian префикс
4. ✅ Парсить timestamp в handleIncomingChunk() — data.slice(8) для MSE
5. ✅ Вычислять incomingRate по скользящему окну wall-time (1 секунда)
6. ✅ Пропускать не-по-порядку блобы (sendTime ≤ prevSendTime)
7. ✅ Переписать секцию 3: feedforward + safeRate = min(incomingRate, safeRate)
8. ✅ Заменить LIVE_RATE_MAX на MIN_RATE/MAX_RATE
9. ✅ Удалить EWMA_ALPHA, liveJitter, liveLastChunkTime, MAX_LATENCY_MS
10. ✅ Обновить документацию
11. ✅ Keyframe detection по размеру блоба (EMA без keyframe, порог 1.5×)
12. ✅ Seek к keyframe при [keyframe] stall; play-only если keyframe не найден