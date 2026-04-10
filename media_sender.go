//go:build !android && !ios

package main

import (
	"fmt"
	"math"
	"time"

	"github.com/pion/mediadevices"
	log "github.com/schollz/logger"
)

// ========== Переменные состояния — аналоги JS ==========

var (
	goSenderActive bool
)

// localStream — аналог JS: var localStream = null
var localStream mediadevices.MediaStream

// mediaRecorder — аналог JS: var mediaRecorder = null
var mediaRecorderInst *mediaRecorder

// recorderStarted — аналог JS: var recorderStarted = false
var recorderStarted bool

// Go sender state для обработки команд от remote peer — аналоги JS peerSettings, recordCodec, lastHandledMpx
var (
	goSenderRoom       *VideoCallRoom         // текущая комната
	goSenderRemotePeer string                 // ID remote пира
	goPeerSettings     map[string]interface{} // последние настройки remote пира (аналог JS peerSettings)
	goRecordCodec      string                 // negotiated record codec (аналог JS recordCodec)
	goLastHandledMpx   float64                // debounce по mpx (аналог JS lastHandledMpx)
)

func detectSupportedCodecs() []string {
	// На десктопе через mediadevices поддерживаем VP8+Opus
	return []string{
		`video/webm;codecs="vp8,opus"`,
		"video/webm",
		`audio/webm;codecs="opus"`,
		"audio/webm",
	}
}

// detectAllRecordCodecs — аналог JS detectAllRecordCodecs()
func detectAllRecordCodecs() []string {
	return []string{
		`video/webm;codecs="vp8,opus"`,
		`video/webm;codecs="vp8"`,
		"video/webm",
		`audio/webm;codecs="opus"`,
		"audio/webm",
	}
}

// negotiateCodec — аналог JS negotiateCodecClient(myRecordCodecs, peerPlayCodecs)
func negotiateCodec(myCodecs, peerCodecs []string) string {
	for _, mc := range myCodecs {
		for _, pc := range peerCodecs {
			if mc == pc {
				return mc
			}
		}
	}
	return ""
}

// detectCodec — аналог JS detectCodec()
func detectCodec() string {
	codecs := detectSupportedCodecs()
	if len(codecs) > 0 {
		return codecs[0]
	}
	return "video/webm"
}

// ========== sendChunk — аналог JS sendChunk(buffer) ==========

var sendChunkFn func(data []byte)

// ========== setupMediaRecorder — аналог JS setupMediaRecorder() ==========

// setupMediaRecorder — аналог JS setupMediaRecorder()
// Создаёт MediaRecorder с нужным кодеком и запускает запись
func setupMediaRecorder(stream mediadevices.MediaStream, codec string, W, H int, onChunk func([]byte)) error {
	rec, err := newMediaRecorder(stream, codec, W, H, onChunk)
	if err != nil {
		return fmt.Errorf("newMediaRecorder: %w", err)
	}
	mediaRecorderInst = rec
	recorderStarted = true
	rec.start(CHUNK_INTERVAL_MS)
	return nil
}

// restartMediaRecorder — аналог JS restartMediaRecorder()
func restartMediaRecorder() {
	if mediaRecorderInst != nil {
		mediaRecorderInst.stop()
	}
	time.Sleep(100 * time.Millisecond)
	if localStream != nil && sendChunkFn != nil {
		codec := detectCodec()
		W, H := 320, 240
		if mediaRecorderInst != nil {
			W, H = mediaRecorderInst.W, mediaRecorderInst.H
		}
		setupMediaRecorder(localStream, codec, W, H, sendChunkFn)
	}
}

// ========== handlePeerSettingsForGoSender / handleRestartRecorderForGoSender ==========
// Аналоги JS handlePeerSettings() — обработка команд от remote peer для Go sender

func toStringSlice(v interface{}) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []interface{}:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

// handlePeerSettingsForGoSender — аналог JS handlePeerSettings(msg, initial=false)
// Вызывается когда remote peer присылает settings JSON через WebSocket.
// Отвечает за: codec negotiation, resolution change (mpx), composition change
func handlePeerSettingsForGoSender(msg map[string]interface{}) {
	if !goSenderActive || localStream == nil || goSenderRoom == nil {
		return
	}

	prev := goPeerSettings
	goPeerSettings = msg

	// === 1. Codec negotiation — аналог JS: negotiateCodecClient(myRecordCodecs, peer.playCodecs) ===
	var codecChanged bool
	if playCodecs := toStringSlice(msg["playCodecs"]); len(playCodecs) > 0 {
		myRec := detectAllRecordCodecs()
		newCodec := negotiateCodec(myRec, playCodecs)
		if newCodec != "" && newCodec != goRecordCodec {
			log.Debugf("GoSender: negotiate record=%s -> %s", goRecordCodec, newCodec)
			goRecordCodec = newCodec
			codecChanged = true
		}
	}

	// === 2. Resolution change (mpx) — аналог JS: findBestResolution(msg.mpx) ===
	var resolutionChanged bool
	mpx := 0.1 // default
	if m, ok := msg["mpx"].(float64); ok && m > 0 {
		mpx = m
	}
	if math.Abs(mpx-goLastHandledMpx) > 0.001 {
		resolutionChanged = true
	}

	// === 3. Composition change — аналог JS: peerWantsMyAudio, peerWantsMyVideo ===
	var compositionChanged bool
	peerWantsMyAudio := true // default
	peerWantsMyVideo := true
	if v, ok := msg["wantAudio"].(bool); ok {
		peerWantsMyAudio = v
	}
	if v, ok := msg["wantVideo"].(bool); ok {
		peerWantsMyVideo = v
	}
	// Go sender всегда имеет audio+video, canSend зависит от желаний пира
	canSend := peerWantsMyAudio || peerWantsMyVideo

	if prev != nil {
		prevWantA := true
		prevWantV := true
		if v, ok := prev["wantAudio"].(bool); ok {
			prevWantA = v
		}
		if v, ok := prev["wantVideo"].(bool); ok {
			prevWantV = v
		}
		prevCanSend := prevWantA || prevWantV
		prevVideoActive := prevWantV
		curVideoActive := peerWantsMyVideo
		if prevCanSend != canSend || prevVideoActive != curVideoActive {
			compositionChanged = true
		}
	}

	// Был ли рекордер уже остановлен?
	wasStopped := !recorderStarted || mediaRecorderInst == nil || mediaRecorderInst.state() == "inactive"

	// === 4. Действия ===

	// Пир ничего не хочет от нас — останавливаем рекордер
	if !canSend {
		if mediaRecorderInst != nil && mediaRecorderInst.state() != "inactive" {
			log.Debugf("GoSender: REC stop — peer doesn't need my streams")
			mediaRecorderInst.stop()
			recorderStarted = false
		}
		return
	}

	needRestart := codecChanged || resolutionChanged || compositionChanged || wasStopped
	if !needRestart {
		return
	}

	if resolutionChanged {
		// Обновляем debounce
		goLastHandledMpx = mpx
		// Полный рестарт: recorder + getUserMedia с новым разрешением
		log.Debugf("GoSender: full restart — resolution changed mpx=%.2f", mpx)
		restartGoSenderWithResolution(mpx)
	} else {
		// Лёгкий рестарт: только recorder (codec/composition change)
		log.Debugf("GoSender: REC restart — codec=%v comp=%v stopped=%v",
			codecChanged, compositionChanged, wasStopped)
		restartMediaRecorder()
	}
}

// restartGoSenderWithResolution — полный рестарт Go sender с новым разрешением
// (закрывает камеру, переоткрывает с новым размером, запускает recorder)
func restartGoSenderWithResolution(mpx float64) {
	// Останавливаем recorder
	if mediaRecorderInst != nil {
		mediaRecorderInst.stop()
		mediaRecorderInst = nil
	}
	recorderStarted = false

	// Закрываем камеру/микрофон
	if localStream != nil {
		for _, t := range localStream.GetTracks() {
			t.Close()
		}
		localStream = nil
	}

	// Ищем новое разрешение — аналог JS: findBestResolution(mpx)
	var W, H int
	if bestW, bestH, ok := findBestResolution(mpx); ok {
		W, H = bestW, bestH
	} else {
		W, H = applyMpx(4, 3, mpx)
		W, H = queryBestCameraResolution(W, H)
	}

	// Переоткрываем камеру/микрофон
	stream, err := getUserMedia(W, H)
	if err != nil {
		log.Debugf("GoSender: getUserMedia restart failed: %v", err)
		goSenderActive = false
		return
	}
	localStream = stream

	// Выбираем кодек
	codec := goRecordCodec
	if codec == "" {
		codec = detectCodec()
	}

	log.Debugf("GoSender: restarted with %dx%d codec=%s", W, H, codec)
	setupMediaRecorder(stream, codec, W, H, sendChunkFn)
}

// handleRestartRecorderForGoSender — обработка "restart_recorder" от remote peer
// Аналог JS: ws.onmessage → "restart_recorder" → restartMediaRecorder()
func handleRestartRecorderForGoSender() {
	if !goSenderActive {
		return
	}
	log.Debugf("GoSender: REC restart requested by peer")
	restartMediaRecorder()
}

// ========== startGoSender / stopGoSender — точки входа (вызываются из videocall.go) ==========

// startGoSender — аналог JS startCall() + setupMediaRecorder()
// Запускает захват камеры/микрофона через mediadevices,
// кодирует VP8+Opus, муксит в WebM чанки и отправляет через WebSocket
func startGoSender(roomID, peerID string, room *VideoCallRoom) error {
	if localStream != nil {
		stopGoSender()
	}

	remotePeer := getRemotePeer(room, peerID)

	// Сохраняем контекст для handlePeerSettingsForGoSender
	goSenderRoom = room
	goSenderRemotePeer = remotePeer

	// Ждём settings от remote пира (таймаут 10 сек)
	settings := room.waitForPeerSettings(remotePeer, 10*time.Second)
	mpx := 0.1 // default — аналог JS: S.mpx = 0.1
	if settings != nil {
		if m, ok := settings["mpx"].(float64); ok && m > 0 {
			mpx = m
			log.Debugf("GoSender: using mpx=%.2f from peer settings", mpx)
		}
	} else {
		log.Debugf("GoSender: no peer settings, using default mpx=%.2f", mpx)
	}

	// Аналог JS: findBestResolution(mpx)
	var W, H int
	if bestW, bestH, ok := findBestResolution(mpx); ok {
		W, H = bestW, bestH
		log.Debugf("GoSender: findBestResolution(%.2f) = %dx%d", mpx, W, H)
	} else {
		// Fallback: applyMpx(aspectW, aspectH, mpx) — аналог JS
		W, H = applyMpx(4, 3, mpx)
		W, H = queryBestCameraResolution(W, H)
		log.Debugf("GoSender: applyMpx fallback %dx%d (mpx=%.2f)", W, H, mpx)
	}

	// Аналог JS: navigator.mediaDevices.getUserMedia(...)
	stream, err := getUserMedia(W, H)
	if err != nil {
		return fmt.Errorf("getUserMedia: %w", err)
	}
	localStream = stream

	// sendChunk callback — аналог JS sendChunk(buffer)
	sendChunkFn = func(data []byte) {
		if !goSenderActive {
			return
		}
		room.addChunk(peerID, data)
		if remotePeer != "" {
			room.forwardChunkToWS(remotePeer, data)
		}
	}

	// Аналог JS: codec = recordCodec || negotiatedCodec || detectCodec()
	codec := detectCodec()

	// Аналог JS: setupMediaRecorder()
	goSenderActive = true
	err = setupMediaRecorder(stream, codec, W, H, sendChunkFn)
	if err != nil {
		goSenderActive = false
		return fmt.Errorf("setupMediaRecorder: %w", err)
	}

	log.Debugf("GoSender started: room=%s peer=%s", roomID, peerID)
	return nil
}

// stopGoSender — аналог JS hangup() + mediaRecorder.stop()
// goLocalPeerSettings — предыдущие настройки локального пира для Go sender
var goLocalPeerSettings map[string]interface{}

// handleLocalPeerSettingsForGoSender обрабатывает настройки от ЛОКАЛЬНОГО пира
// Сравнивает предыдущие и новые значения audio/video/goSender и выполняет действия:
// - goSender false→true: запуск Go sender
// - goSender true→false: остановка Go sender
// - video/audio changed: остановка/запуск соответствующих capture devices
func handleLocalPeerSettingsForGoSender(msg map[string]interface{}, roomID, peerID string, room *VideoCallRoom) {
	prev := goLocalPeerSettings
	goLocalPeerSettings = msg

	// Извлекаем текущие значения
	goSender := false
	audio := true
	video := true
	if v, ok := msg["goSender"].(bool); ok {
		goSender = v
	}
	if v, ok := msg["audio"].(bool); ok {
		audio = v
	}
	if v, ok := msg["video"].(bool); ok {
		video = v
	}

	// Извлекаем предыдущие значения
	prevGoSender := false
	prevAudio := true
	prevVideo := true
	if prev != nil {
		if v, ok := prev["goSender"].(bool); ok {
			prevGoSender = v
		}
		if v, ok := prev["audio"].(bool); ok {
			prevAudio = v
		}
		if v, ok := prev["video"].(bool); ok {
			prevVideo = v
		}
	}

	log.Debugf("GoSender local settings: goSender=%v->%v audio=%v->%v video=%v->%v active=%v",
		prevGoSender, goSender, prevAudio, audio, prevVideo, video, goSenderActive)

	// === 1. goSender toggle ===
	if goSender && !prevGoSender && !goSenderActive {
		// Go sender ON: запуск
		go func() {
			if err := startGoSender(roomID, peerID, room); err != nil {
				log.Debugf("GoSender start via settings failed: %v", err)
			}
		}()
		return
	}
	if !goSender && prevGoSender && goSenderActive {
		// Go sender OFF: полная остановка
		stopGoSender()
		return
	}

	// Если Go sender не активен — больше ничего не делаем
	if !goSenderActive {
		return
	}

	// === 2. Video toggle ===
	if prevVideo && !video {
		// Видео выключено: рестарт recorder (audio-only) сразу, release камеры с задержкой 2 сек
		log.Debugf("GoSender: video OFF — scheduling camera release (2s delay)")
		// Рестарт recorder сразу (audio-only если audio включен)
		if !audio {
			// Всё выключено — стоп рекордер
			if mediaRecorderInst != nil && mediaRecorderInst.state() != "inactive" {
				mediaRecorderInst.stop()
				recorderStarted = false
			}
		} else {
			restartMediaRecorder()
		}
		// Release камеры с задержкой 2 сек (чтоб у пира не завис последний кадр)
		go func() {
			time.Sleep(2 * time.Second)
			if localStream != nil {
				for _, t := range localStream.GetVideoTracks() {
					t.Close()
				}
				log.Debugf("GoSender: camera released (delayed)")
			}
		}()
	} else if !prevVideo && video {
		// Видео включено: переоткрыть камеру
		log.Debugf("GoSender: video ON — re-acquiring camera")
		go restartGoSenderTrack("video", audio, video)
	}

	// === 3. Audio toggle ===
	if prevAudio && !audio {
		// Аудио выключено: закрыть audio tracks
		log.Debugf("GoSender: audio OFF — releasing microphone")
		if localStream != nil {
			for _, t := range localStream.GetAudioTracks() {
				t.Close()
			}
		}
		if !video {
			// Всё выключено — стоп рекордер
			if mediaRecorderInst != nil && mediaRecorderInst.state() != "inactive" {
				mediaRecorderInst.stop()
				recorderStarted = false
			}
		} else {
			restartMediaRecorder()
		}
	} else if !prevAudio && audio {
		// Аудио включено: переоткрыть микрофон
		log.Debugf("GoSender: audio ON — re-acquiring microphone")
		go restartGoSenderTrack("audio", audio, video)
	}
}

// restartGoSenderTrack переоткрывает указанный track (video или audio)
// и перезапускает media recorder
func restartGoSenderTrack(trackType string, wantAudio, wantVideo bool) {
	if localStream == nil {
		return
	}

	// Останавливаем recorder
	if mediaRecorderInst != nil {
		mediaRecorderInst.stop()
		recorderStarted = false
	}

	// Закрываем все треки и освобождаем stream
	for _, t := range localStream.GetTracks() {
		t.Close()
	}
	localStream = nil

	// Заново получаем медиа
	if !wantAudio && !wantVideo {
		return
	}

	// Определяем разрешение
	var W, H int
	if wantVideo {
		mpx := 0.1
		if goPeerSettings != nil {
			if m, ok := goPeerSettings["mpx"].(float64); ok && m > 0 {
				mpx = m
			}
		}
		if bestW, bestH, ok := findBestResolution(mpx); ok {
			W, H = bestW, bestH
		} else {
			W, H = applyMpx(4, 3, mpx)
			W, H = queryBestCameraResolution(W, H)
		}
	}

	stream, err := getUserMedia(W, H)
	if err != nil {
		log.Debugf("GoSender: getUserMedia restart failed: %v", err)
		goSenderActive = false
		return
	}
	localStream = stream

	// Запускаем recorder
	codec := goRecordCodec
	if codec == "" {
		codec = detectCodec()
	}
	setupMediaRecorder(stream, codec, W, H, sendChunkFn)
}

func stopGoSender() {
	// Аналог JS: if (mediaRecorder && mediaRecorder.state !== 'inactive') mediaRecorder.stop()
	if mediaRecorderInst != nil {
		mediaRecorderInst.stop()
		mediaRecorderInst = nil
	}
	recorderStarted = false
	goSenderActive = false

	// Аналог JS: if (localStream) localStream.getTracks().forEach(t => t.stop())
	if localStream != nil {
		for _, t := range localStream.GetTracks() {
			t.Close()
		}
		localStream = nil
	}
	sendChunkFn = nil
}
