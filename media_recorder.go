//go:build !android && !ios

package main

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/at-wat/ebml-go/webm"
	"github.com/pion/mediadevices"
	log "github.com/schollz/logger"
)


// ========== Константы-аналоги JS ==========

var CHUNK_INTERVAL_MS = 100 // как в JS: mediaRecorder.start(CHUNK_INTERVAL_MS)


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
	stream  mediadevices.MediaStream // аналог JS localStream
	encV    mediadevices.EncodedReadCloser
	encA    mediadevices.EncodedReadCloser
	done    chan struct{}
	active  bool
	W, H    int
	codec   string
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
		active:  false,
		W:       W,
		H:       H,
		codec:   codec,
		onChunk: onChunk,
		buf:     &syncedBuffer{},
	}, nil
}

// start — аналог JS: mediaRecorder.start(CHUNK_INTERVAL_MS)
func (mr *mediaRecorder) start(intervalMs int) {
	if mr.active {
		return
	}
	mr.active = true

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
		return
	}

	ws, err := webm.NewSimpleBlockWriter(mr.buf, tracks,
		mkvcore.WithBlockInterceptor(sorter),
		mkvcore.WithMaxKeyframeInterval(1, 30),
	)
	if err != nil {
		log.Debugf("MediaRecorder WebM writer error: %v", err)
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
		defer func() {
			mr.active = false
		}()

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
							return
						}
						ts := f.timestampMS
						videoWriter.Write(true, ts, f.data)
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
							return
						}
						ts := f.timestampMS
						audioWriter.Write(true, ts, f.data)
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
	if !mr.active {
		return
	}
	mr.active = false
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
	if mr == nil || !mr.active {
		return "inactive"
	}
	return "recording"
}

// ========== Camera capabilities — аналоги JS getCameraCapabilities / findBestResolution / applyMpx ==========

// getCameraCapabilities — аналог JS getCameraCapabilities()
