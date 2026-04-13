//go:build !android && !ios

package main

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/at-wat/ebml-go/webm"
	"github.com/pion/mediadevices"
	log "github.com/schollz/logger"
)

// ========== Константы-аналоги JS ==========

const CHUNK_INTERVAL_MS = 100 // как в JS: mediaRecorder.start(CHUNK_INTERVAL_MS)

type syncedBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *syncedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.data = append(b.data, p...)
	b.mu.Unlock()
	return len(p), nil
}

func (b *syncedBuffer) take() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.data
	b.data = nil
	return d
}

func (b *syncedBuffer) Close() error { return nil }

// ========== frameData ==========

type frameData struct {
	data        []byte
	samples     uint32
	timestampMS int64 // время чтения из энкодера — максимально близко к моменту захвата
}

// ========== mediaRecorder — аналог JS MediaRecorder ==========

// mediaRecorder — Go-аналог JS new MediaRecorder(localStream, {mimeType: codec})
type mediaRecorder struct {
	mu      sync.Mutex               // Fix 3: protects stream/encV/encA/W/H mutations
	stream  mediadevices.MediaStream // аналог JS localStream
	encV    mediadevices.EncodedReadCloser
	encA    mediadevices.EncodedReadCloser
	done    chan struct{}
	active  atomic.Bool
	W, H    int
	codec   string            // Fix 4: stored for logging/future use; actual codec is hardcoded as VP8+Opus in track entries
	onChunk func(data []byte) // аналог JS ondataavailable
	buf     *syncedBuffer
}

// newMediaRecorder — аналог JS: mediaRecorder = new MediaRecorder(localStream, { mimeType: codec })
func newMediaRecorder(stream mediadevices.MediaStream, codec string, W, H int, onChunk func([]byte)) (*mediaRecorder, error) {
	var videoTrack, audioTrack mediadevices.Track
	for _, t := range stream.GetTracks() {
		switch t.Kind().String() {
		case "video":
			videoTrack = t
		case "audio":
			audioTrack = t
		}
	}
	if videoTrack == nil {
		return nil, fmt.Errorf("no video track available")
	}
	if audioTrack == nil {
		return nil, fmt.Errorf("no audio track available")
	}

	encV, err := videoTrack.NewEncodedReader("video/vp8")
	if err != nil {
		return nil, fmt.Errorf("NewEncodedReader video: %w", err)
	}

	encA, err := audioTrack.NewEncodedReader("audio/opus")
	if err != nil {
		encV.Close()
		return nil, fmt.Errorf("NewEncodedReader audio: %w", err)
	}

	return &mediaRecorder{
		stream:  stream,
		encV:    encV,
		encA:    encA,
		done:    make(chan struct{}),
		W:       W,
		H:       H,
		codec:   codec,
		onChunk: onChunk,
		buf:     &syncedBuffer{},
	}, nil
}

// start — аналог JS: mediaRecorder.start(CHUNK_INTERVAL_MS)
func (mr *mediaRecorder) start(intervalMs int) {
	if mr.active.Load() {
		return
	}
	mr.active.Store(true)

	// Горутина чтения видео
	var totalVideoSamples uint64
	videoCh := make(chan frameData, 10)
	go func() {
		defer close(videoCh)
		for {
			encoded, release, err := mr.encV.Read()
			if err != nil {
				if err != io.EOF {
					log.Debugf("MediaRecorder video read: %v", err)
				}
				return
			}
			data := make([]byte, len(encoded.Data))
			copy(data, encoded.Data)
			totalVideoSamples += uint64(encoded.Samples)
			timestampMS := int64(totalVideoSamples * 1000 / 90000)
			release()
			select {
			case videoCh <- frameData{data: data, samples: encoded.Samples, timestampMS: timestampMS}:
			case <-mr.done:
				return
			}
		}
	}()

	// Горутина чтения аудио
	var totalAudioSamples uint64
	audioCh := make(chan frameData, 30)
	go func() {
		defer close(audioCh)
		for {
			encoded, release, err := mr.encA.Read()
			if err != nil {
				if err != io.EOF {
					log.Debugf("MediaRecorder audio read: %v", err)
				}
				return
			}
			data := make([]byte, len(encoded.Data))
			copy(data, encoded.Data)
			totalAudioSamples += uint64(encoded.Samples)
			timestampMS := int64(totalAudioSamples * 1000 / 48000)
			release()
			select {
			case audioCh <- frameData{data: data, samples: encoded.Samples, timestampMS: timestampMS}:
			case <-mr.done:
				return
			}
		}
	}()

	// WebM tracks — аналог JS MediaRecorder {mimeType} + Video/Audio specification
	tracks := []webm.TrackEntry{
		{
			Name:            "Video",
			TrackNumber:     1,
			TrackUID:        1,
			CodecID:         "V_VP8",
			TrackType:       1,
			DefaultDuration: 33333333, // ~30fps
			Video: &webm.Video{
				PixelWidth:  uint64(mr.W),
				PixelHeight: uint64(mr.H),
			},
		},
		{
			Name:            "Audio",
			TrackNumber:     2,
			TrackUID:        2,
			CodecID:         "A_OPUS",
			TrackType:       2,
			DefaultDuration: 20000000, // 20ms
			Audio: &webm.Audio{
				SamplingFrequency: 48000,
				Channels:          1,
			},
		},
	}

	sorter, err := mkvcore.NewMultiTrackBlockSorter(
		mkvcore.WithMaxTimescaleDelay(100),
	)
	if err != nil {
		log.Debugf("MediaRecorder block sorter error: %v", err)
		close(mr.done)
		mr.active.Store(false)
		// Fix 1b: close encoders — read goroutines already launched above
		if mr.encV != nil {
			mr.encV.Close()
		}
		if mr.encA != nil {
			mr.encA.Close()
		}
		return
	}

	ws, err := webm.NewSimpleBlockWriter(mr.buf, tracks,
		mkvcore.WithBlockInterceptor(sorter),
		mkvcore.WithMaxKeyframeInterval(1, 30),
	)
	if err != nil {
		log.Debugf("MediaRecorder WebM writer error: %v", err)
		close(mr.done)
		mr.active.Store(false)
		// Fix 1b: close encoders — read goroutines already launched above
		if mr.encV != nil {
			mr.encV.Close()
		}
		if mr.encA != nil {
			mr.encA.Close()
		}
		return
	}
	videoWriter := ws[0]
	audioWriter := ws[1]

	// Init segment — отправляем первым (аналог JS: первый чанк от MediaRecorder)
	if initSeg := mr.buf.take(); len(initSeg) > 0 && mr.onChunk != nil {
		mr.onChunk(initSeg)
		log.Debugf("MediaRecorder sent init segment (%d bytes)", len(initSeg))
	}

	// Главная горутина: WebM муксинг + отправка media segments
	go func() {
		ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-mr.done:
				return
			case <-ticker.C:
				// Записываем видео-фреймы (неблокирующе)
				// Таймстамп = реальное время захвата относительно начала стрима
			readVideo:
				for {
					select {
					case f, ok := <-videoCh:
						if !ok {
							mr.stop() // Fix: clean shutdown on unexpected channel close
							return
						}
						ts := f.timestampMS
						if _, err := videoWriter.Write(true, ts, f.data); err != nil {
							log.Debugf("MediaRecorder video write error: %v", err)
							mr.stop() // Fix: clean shutdown on write error
							return
						}
					default:
						break readVideo
					}
				}
				// Записываем аудио-фреймы (неблокирующе)
				// Таймстамп по той же шкале что и видео
			readAudio:
				for {
					select {
					case f, ok := <-audioCh:
						if !ok {
							mr.stop() // Fix: clean shutdown on unexpected channel close
							return
						}
						ts := f.timestampMS
						if _, err := audioWriter.Write(true, ts, f.data); err != nil {
							log.Debugf("MediaRecorder audio write error: %v", err)
							mr.stop() // Fix: clean shutdown on write error
							return
						}
					default:
						break readAudio
					}
				}
				// Забираем накопленный media segment и отправляем — аналог ondataavailable
				if data := mr.buf.take(); len(data) > 0 && mr.onChunk != nil {
					mr.onChunk(data)
				}
			}
		}
	}()

	log.Debugf("MediaRecorder started: codec=%s res=%dx%d interval=%dms", mr.codec, mr.W, mr.H, intervalMs)
}

// stop — аналог JS: mediaRecorder.stop()
func (mr *mediaRecorder) stop() {
	if !mr.active.Load() {
		return
	}
	mr.active.Store(false)
	select {
	case <-mr.done:
		// already closed
	default:
		close(mr.done)
	}
	if mr.encV != nil {
		mr.encV.Close()
		mr.encV = nil
	}
	if mr.encA != nil {
		mr.encA.Close()
		mr.encA = nil
	}
}

// state — аналог JS: mediaRecorder.state
func (mr *mediaRecorder) state() string {
	if mr == nil || !mr.active.Load() {
		return "inactive"
	}
	return "recording"
}

// ========== Динамическое разрешение — аналог JS applyDesiredResolution ==========

// applyDesiredResolution — аналог JS applyDesiredResolution(dw, dh)
// Пересоздаёт stream с новым разрешением (pion не поддерживает hot-swap constraints)
func (mr *mediaRecorder) applyDesiredResolution(newW, newH int) error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if newW == mr.W && newH == mr.H {
		return nil // разрешение не изменилось
	}

	wasActive := mr.active.Load()

	// 1. Останавливаем текущий рекордер
	if wasActive {
		mr.stop()
	}

	// 2. Создаём новый stream с новым разрешением ДО закрытия старого (Fix 2: rollback safety)
	newStream, err := getUserMedia(newW, newH)
	if err != nil {
		return fmt.Errorf("getUserMedia %dx%d: %w", newW, newH, err)
	}

	// 3. Находим новые tracks
	var videoTrack, audioTrack mediadevices.Track
	for _, t := range newStream.GetTracks() {
		switch t.Kind().String() {
		case "video":
			videoTrack = t
		case "audio":
			audioTrack = t
		}
	}
	if videoTrack == nil {
		for _, t := range newStream.GetTracks() {
			t.Close()
		}
		return fmt.Errorf("no video track in new stream")
	}
	if audioTrack == nil {
		for _, t := range newStream.GetTracks() {
			t.Close()
		}
		return fmt.Errorf("no audio track in new stream")
	}

	// 4. Новые энкодеры
	encV, err := videoTrack.NewEncodedReader("video/vp8")
	if err != nil {
		for _, t := range newStream.GetTracks() {
			t.Close()
		}
		return fmt.Errorf("NewEncodedReader video: %w", err)
	}

	encA, err := audioTrack.NewEncodedReader("audio/opus")
	if err != nil {
		encV.Close()
		for _, t := range newStream.GetTracks() {
			t.Close()
		}
		return fmt.Errorf("NewEncodedReader audio: %w", err)
	}

	// 5. Закрываем старые tracks (только после успешного создания нового stream)
	if mr.stream != nil {
		for _, t := range mr.stream.GetTracks() {
			t.Close()
		}
	}

	// 6. Обновляем поля
	mr.stream = newStream
	mr.encV = encV
	mr.encA = encA
	mr.W = newW
	mr.H = newH

	// 7. Перезапускаем если был активен
	if wasActive {
		mr.done = make(chan struct{})
		mr.start(CHUNK_INTERVAL_MS)
	}

	log.Debugf("MediaRecorder resolution changed to %dx%d", newW, newH)
	return nil
}

// applyPeerMpx — аналог JS handlePeerSettings: находит лучшее разрешение по mpx пира
// Note: does NOT lock mr.mu — delegates to applyDesiredResolution which holds the lock
func (mr *mediaRecorder) applyPeerMpx(mpx float64) error {
	if mpx <= 0 {
		return nil
	}

	bestW, bestH, ok := findBestResolution(mpx)
	if ok && bestW > 0 && bestH > 0 {
		log.Debugf("PEER wants %.2fmpx → best=%dx%d", mpx, bestW, bestH)
		return mr.applyDesiredResolution(bestW, bestH)
	}

	// Fallback: applyMpx от текущего аспекта
	w, h := applyMpx(mr.W, mr.H, mpx)
	log.Debugf("PEER wants %.2fmpx → fallback=%dx%d", mpx, w, h)
	return mr.applyDesiredResolution(w, h)
}

// ========== Camera capabilities — аналоги JS getCameraCapabilities / findBestResolution / applyMpx ==========

// getCameraCapabilities — аналог JS getCameraCapabilities()
