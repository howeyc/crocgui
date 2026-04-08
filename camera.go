//go:build !android && !ios

package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	"github.com/pion/mediadevices/pkg/driver"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	"github.com/pion/mediadevices/pkg/prop"
	log "github.com/schollz/logger"
)

func getCameraCapabilities() (minW, maxW, stepW, minH, maxH, stepH int, ok bool) {
	typeFilter := driver.FilterVideoRecorder()
	notScreenFilter := driver.FilterNot(driver.FilterDeviceType(driver.Screen))
	filter := driver.FilterAnd(typeFilter, notScreenFilter)

	drivers := driver.GetManager().Query(filter)
	for _, d := range drivers {
		needClose := false
		if d.Status() == driver.StateClosed {
			if err := d.Open(); err != nil {
				continue
			}
			needClose = true
		}

		props := d.Properties()
		if needClose {
			d.Close()
		}

		if len(props) == 0 {
			continue
		}

		minW, minH = math.MaxInt32, math.MaxInt32
		maxW, maxH = 0, 0
		for _, p := range props {
			if p.Width == 0 || p.Height == 0 {
				continue
			}
			if p.Width < minW {
				minW = p.Width
			}
			if p.Width > maxW {
				maxW = p.Width
			}
			if p.Height < minH {
				minH = p.Height
			}
			if p.Height > maxH {
				maxH = p.Height
			}
		}
		stepW = detectStep(props, true)
		stepH = detectStep(props, false)
		if stepW == 0 {
			stepW = 1
		}
		if stepH == 0 {
			stepH = 1
		}
		log.Debugf("getCameraCapabilities: %d-%dx%d-%d step=%dx%d (%d modes)",
			minW, maxW, minH, maxH, stepW, stepH, len(props))
		return minW, maxW, stepW, minH, maxH, stepH, true
	}
	return 0, 0, 1, 0, 0, 1, false
}

// detectStep — вспомогательная для определения минимального шага между соседними значениями
func detectStep(props []prop.Media, width bool) int {
	vals := make(map[int]struct{})
	for _, p := range props {
		v := p.Width
		if !width {
			v = p.Height
		}
		if v > 0 {
			vals[v] = struct{}{}
		}
	}
	if len(vals) < 2 {
		return 1
	}
	sorted := make([]int, 0, len(vals))
	for v := range vals {
		sorted = append(sorted, v)
	}
	sort.Ints(sorted)
	minStep := sorted[len(sorted)-1] - sorted[0]
	for i := 1; i < len(sorted); i++ {
		step := sorted[i] - sorted[i-1]
		if step > 0 && step < minStep {
			minStep = step
		}
	}
	return minStep
}

// applyMpx — точная копия JS: applyMpx(aspectW, aspectH, mpx)
// Масштабирование разрешения по абсолютным мегапикселям
func applyMpx(aspectW, aspectH int, mpx float64) (int, int) {
	targetPixels := mpx * 1000000
	aspectRatio := float64(aspectW) / float64(aspectH)
	h := int(math.Round(math.Sqrt(targetPixels / aspectRatio)))
	w := int(math.Round(float64(h) * aspectRatio))
	if w%2 != 0 {
		w++
	}
	if h%2 != 0 {
		h++
	}
	return w, h
}

// findBestResolution — аналог JS findBestResolution(mpx)
// Находит максимальное нативное разрешение камеры <= mpx мегапикселей
// Бинарный поиск (дихотомия) по getCameraCapabilities, fallback на applyMpx
func findBestResolution(mpx float64) (int, int, bool) {
	minW, maxW, stepW, minH, maxH, stepH, ok := getCameraCapabilities()
	if !ok || maxW == 0 || maxH == 0 {
		return 0, 0, false
	}

	aspect := float64(maxW) / float64(maxH)
	maxPixels := mpx * 1000000

	// Вспомогательная: пиксели при данной ширине w (с учётом аспекта и step)
	pixelsAt := func(w int) (int, int, int) {
		h := int(math.Round(float64(w)/aspect/float64(stepH))) * stepH
		if h < minH {
			h = minH
		}
		if h > maxH {
			h = maxH
		}
		return w, h, w * h
	}

	// Если даже minW превышает лимит — возвращаем минимум
	_, _, minPx := pixelsAt(minW)
	if minPx > int(maxPixels) {
		w, h, _ := pixelsAt(minW)
		return w, h, true
	}

	// Бинарный поиск: f(w) монотонно возрастает → ищем max w где w*h <= maxPixels
	lo, hi := minW, maxW
	bestW, bestH := minW, minH

	for hi-lo >= stepW {
		mid := int(math.Round(float64(lo+hi)/2/float64(stepW))) * stepW
		if mid < lo {
			mid = lo
		}
		if mid > hi {
			mid = hi
		}
		if mid == lo && lo+stepW <= hi {
			mid = lo + stepW
		}
		w, h, px := pixelsAt(mid)
		if px <= int(maxPixels) {
			bestW, bestH = w, h
			lo = mid + stepW
		} else {
			hi = mid - stepW
		}
	}
	if bestW%2 != 0 {
		bestW = int(math.Max(float64(minW), float64(bestW-stepW)))
	}
	if bestH%2 != 0 {
		bestH = int(math.Max(float64(minH), float64(bestH-stepH)))
	}
	return bestW, bestH, true
}

// queryBestCameraResolution — fallback для findBestResolution:
// запрашивает список режимов камеры и находит ближайший к (targetW, targetH)
func queryBestCameraResolution(targetW, targetH int) (int, int) {
	typeFilter := driver.FilterVideoRecorder()
	notScreenFilter := driver.FilterNot(driver.FilterDeviceType(driver.Screen))
	filter := driver.FilterAnd(typeFilter, notScreenFilter)

	drivers := driver.GetManager().Query(filter)
	for _, d := range drivers {
		needClose := false
		if d.Status() == driver.StateClosed {
			if err := d.Open(); err != nil {
				continue
			}
			needClose = true
		}

		props := d.Properties()
		if needClose {
			d.Close()
		}

		if len(props) == 0 {
			continue
		}

		bestW, bestH := targetW, targetH
		bestDist := math.MaxFloat64
		for _, p := range props {
			if p.Width == 0 || p.Height == 0 {
				continue
			}
			dw := math.Abs(float64(p.Width - targetW))
			dh := math.Abs(float64(p.Height - targetH))
			dist := dw + dh
			if dist < bestDist {
				bestDist = dist
				bestW = p.Width
				bestH = p.Height
			}
		}
		log.Debugf("queryBestCameraResolution: requested %dx%d, best match %dx%d (from %d modes)",
			targetW, targetH, bestW, bestH, len(props))
		return bestW, bestH
	}

	log.Debugf("queryBestCameraResolution: no camera found, using %dx%d", targetW, targetH)
	return targetW, targetH
}

// ========== getUserMedia — аналог navigator.mediaDevices.getUserMedia ==========

// getUserMedia — аналог JS: navigator.mediaDevices.getUserMedia({video: ..., audio: ...})
func getUserMedia(W, H int) (mediadevices.MediaStream, error) {
	vp8p, err := vpx.NewVP8Params()
	if err != nil {
		return nil, fmt.Errorf("VP8 params: %w", err)
	}
	vp8p.BitRate = 500_000 // 500 kbps

	opusp, err := opus.NewParams()
	if err != nil {
		return nil, fmt.Errorf("Opus params: %w", err)
	}

	selector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&vp8p),
		mediadevices.WithAudioEncoders(&opusp),
	)

	return mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.Width = prop.Int(W)
			c.Height = prop.Int(H)
		},
		Audio: func(c *mediadevices.MediaTrackConstraints) {},
		Codec: selector,
	})
}

// ========== detectSupportedCodecs — аналог JS detectSupportedCodecs / detectAllRecordCodecs ==========
