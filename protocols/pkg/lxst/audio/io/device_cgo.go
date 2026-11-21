//go:build cgo && (linux || darwin || windows || android || freebsd || openbsd || netbsd || dragonfly)

// SPDX-License-Identifier: LicenseRef-Reticulum
//
//revive:disable:var-naming
package io

/*
#cgo CFLAGS: -I${SRCDIR}/../../../../third_party/miniaudio -O3
#cgo linux,musl LDFLAGS: -lm -lpthread
#cgo linux,!musl LDFLAGS: -lm -lpthread -ldl
#cgo linux,arm LDFLAGS: -latomic
#cgo darwin LDFLAGS: -lm -lpthread
#cgo windows LDFLAGS: -lm
#cgo android LDFLAGS: -lm -lOpenSLES
#cgo android,arm LDFLAGS: -latomic
#cgo freebsd openbsd netbsd dragonfly LDFLAGS: -lm -lpthread
#cgo freebsd,arm LDFLAGS: -latomic
#cgo openbsd,arm LDFLAGS: -latomic
#cgo netbsd,arm LDFLAGS: -latomic
#cgo dragonfly,arm LDFLAGS: -latomic

#define MINIAUDIO_IMPLEMENTATION
#include "miniaudio.h"
#include <stdlib.h>
#include <string.h>

typedef struct {
	ma_device device;
	ma_device_config config;
	ma_device_id play_id;
	ma_device_id cap_id;
	int have_play;
	int have_cap;
	ma_pcm_rb capture_rb;
	ma_pcm_rb playback_rb;
	int frameSize;
	int started;
	int duplex;
} rgesp_audio;

static void rgesp_data_cb(ma_device *pDevice, void *pOutput, const void *pInput, ma_uint32 frameCount) {
	rgesp_audio *a = (rgesp_audio *)pDevice->pUserData;
	if (pInput != NULL) {
		void *dst = NULL;
		ma_uint32 avail = frameCount;
		if (ma_pcm_rb_acquire_write(&a->capture_rb, &avail, &dst) == MA_SUCCESS) {
			memcpy(dst, pInput, avail * ma_get_bytes_per_frame(pDevice->capture.format, pDevice->capture.channels));
			ma_pcm_rb_commit_write(&a->capture_rb, avail);
		}
	}
	if (pOutput != NULL) {
		void *src = NULL;
		ma_uint32 avail = frameCount;
		ma_uint32 got = 0;
		size_t bpf = ma_get_bytes_per_frame(pDevice->playback.format, pDevice->playback.channels);
		if (ma_pcm_rb_acquire_read(&a->playback_rb, &avail, &src) == MA_SUCCESS && src != NULL) {
			memcpy(pOutput, src, avail * bpf);
			ma_pcm_rb_commit_read(&a->playback_rb, avail);
			got = avail;
		}
		if (got < frameCount) {
			memset((unsigned char *)pOutput + got * bpf, 0, (frameCount - got) * bpf);
		}
	}
}

static int rgesp_contains_ci(const char *hay, const char *needle) {
	size_t n;
	size_t i;
	if (needle == NULL || needle[0] == 0) {
		return 1;
	}
	n = strlen(needle);
	for (i = 0; hay[i] != 0; i++) {
		size_t k = 0;
		while (k < n) {
			char a = hay[i + k];
			char b = needle[k];
			if (a >= 'A' && a <= 'Z') {
				a = (char)(a - 'A' + 'a');
			}
			if (b >= 'A' && b <= 'Z') {
				b = (char)(b - 'A' + 'a');
			}
			if (a == 0 || a != b) {
				break;
			}
			k++;
		}
		if (k == n) {
			return 1;
		}
	}
	return 0;
}

static int rgesp_find_device_id(int capture, const char *want, ma_device_id *out) {
	ma_context ctx;
	ma_device_info *play = NULL;
	ma_device_info *cap = NULL;
	ma_uint32 nplay = 0;
	ma_uint32 ncap = 0;
	ma_uint32 i;
	if (want == NULL || want[0] == 0) {
		return 0;
	}
	if (ma_context_init(NULL, 0, NULL, &ctx) != MA_SUCCESS) {
		return 0;
	}
	if (ma_context_get_devices(&ctx, &play, &nplay, &cap, &ncap) != MA_SUCCESS) {
		ma_context_uninit(&ctx);
		return 0;
	}
	if (capture) {
		for (i = 0; i < ncap; i++) {
			if (rgesp_contains_ci(cap[i].name, want)) {
				*out = cap[i].id;
				ma_context_uninit(&ctx);
				return 1;
			}
		}
	} else {
		for (i = 0; i < nplay; i++) {
			if (rgesp_contains_ci(play[i].name, want)) {
				*out = play[i].id;
				ma_context_uninit(&ctx);
				return 1;
			}
		}
	}
	ma_context_uninit(&ctx);
	return 0;
}

static int rgesp_audio_init_ex(rgesp_audio *a, int role, const char *play_name, const char *cap_name) {
	ma_device_config config;
	ma_device_id play_id;
	ma_device_id cap_id;
	int have_play = 0;
	int have_cap = 0;
	ma_device_type dtype;
	a->frameSize = 960;
	a->duplex = (role == 2);
	if (role == 1) {
		dtype = ma_device_type_playback;
	} else if (role == 2) {
		dtype = ma_device_type_duplex;
	} else {
		dtype = ma_device_type_capture;
	}
	config = ma_device_config_init(dtype);
	config.sampleRate = 48000;
	config.periodSizeInFrames = 960;
	config.dataCallback = rgesp_data_cb;
	config.pUserData = a;
	if (dtype != ma_device_type_playback) {
		config.capture.format = ma_format_s16;
		config.capture.channels = 1;
		have_cap = rgesp_find_device_id(1, cap_name, &cap_id);
		if (have_cap) {
			config.capture.pDeviceID = &cap_id;
		}
	}
	if (dtype != ma_device_type_capture) {
		config.playback.format = ma_format_s16;
		config.playback.channels = 1;
		have_play = rgesp_find_device_id(0, play_name, &play_id);
		if (have_play) {
			config.playback.pDeviceID = &play_id;
		}
	}
	if (ma_pcm_rb_init(ma_format_s16, 1, 960 * 8, NULL, NULL, &a->capture_rb) != MA_SUCCESS) {
		return -1;
	}
	if (dtype != ma_device_type_capture) {
		if (ma_pcm_rb_init(ma_format_s16, 1, 960 * 16, NULL, NULL, &a->playback_rb) != MA_SUCCESS) {
			ma_pcm_rb_uninit(&a->capture_rb);
			return -1;
		}
		a->duplex = 1;
	}
	if (ma_device_init(NULL, &config, &a->device) != MA_SUCCESS) {
		ma_pcm_rb_uninit(&a->capture_rb);
		if (a->duplex) {
			ma_pcm_rb_uninit(&a->playback_rb);
		}
		return -1;
	}
	a->config = config;
	a->have_cap = have_cap;
	a->have_play = have_play;
	if (have_cap) {
		a->cap_id = cap_id;
	}
	if (have_play) {
		a->play_id = play_id;
	}
	a->started = 0;
	return 0;
}

static int rgesp_audio_reinit(rgesp_audio *a) {
	if (a->started) {
		ma_device_stop(&a->device);
		a->started = 0;
	}
	ma_device_uninit(&a->device);
	a->config.capture.pDeviceID = a->have_cap ? &a->cap_id : NULL;
	a->config.playback.pDeviceID = a->have_play ? &a->play_id : NULL;
	if (ma_device_init(NULL, &a->config, &a->device) != MA_SUCCESS) {
		return -1;
	}
	ma_pcm_rb_reset(&a->capture_rb);
	if (ma_device_start(&a->device) != MA_SUCCESS) {
		return -1;
	}
	a->started = 1;
	return 0;
}

static int rgesp_audio_start(rgesp_audio *a) {
	if (a->started) {
		return 0;
	}
	if (ma_device_start(&a->device) != MA_SUCCESS) {
		return -1;
	}
	a->started = 1;
	return 0;
}

static int rgesp_audio_read(rgesp_audio *a, short *out, int frames) {
	void *src = NULL;
	ma_uint32 avail = (ma_uint32)frames;
	if (ma_pcm_rb_acquire_read(&a->capture_rb, &avail, &src) != MA_SUCCESS || src == NULL) {
		return 0;
	}
	memcpy(out, src, avail * sizeof(short));
	ma_pcm_rb_commit_read(&a->capture_rb, avail);
	return (int)avail;
}

static int rgesp_audio_write(rgesp_audio *a, short *in, int frames) {
	void *dst = NULL;
	ma_uint32 avail;
	if (!a->duplex || in == NULL || frames <= 0) {
		return 0;
	}
	avail = (ma_uint32)frames;
	if (ma_pcm_rb_acquire_write(&a->playback_rb, &avail, &dst) != MA_SUCCESS || dst == NULL) {
		return 0;
	}
	memcpy(dst, in, avail * sizeof(short));
	ma_pcm_rb_commit_write(&a->playback_rb, avail);
	return (int)avail;
}

static void rgesp_audio_close(rgesp_audio *a) {
	if (a->started) {
		ma_device_stop(&a->device);
		a->started = 0;
	}
	ma_device_uninit(&a->device);
	ma_pcm_rb_uninit(&a->capture_rb);
	if (a->duplex) {
		ma_pcm_rb_uninit(&a->playback_rb);
	}
}
*/
import "C"
import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// checkCaptureSignal enables the dead-input watchdog from upstream LXST
// 0.6.0, which only runs on Android where the recorder can silently die.
const checkCaptureSignal = runtime.GOOS == "android"

// signalFailReinitThreshold matches upstream
// LineSource.SIGNAL_FAIL_REINIT_THRESHOLD.
const signalFailReinitThreshold = 1500 * time.Millisecond

type miniaudioDevice struct {
	dev       *C.rgesp_audio
	duplex    bool
	closed    bool
	frameSize int
	pcm       []int16

	checkSignal bool
	signalOK    bool
	zeroSince   time.Time
}

func Open(opts Options) (Device, error) {
	d := &miniaudioDevice{
		duplex:      opts.Role != RoleCapture,
		frameSize:   DefaultFrameSize,
		checkSignal: checkCaptureSignal && opts.Role != RolePlayback,
	}
	d.dev = (*C.rgesp_audio)(C.calloc(1, C.sizeof_rgesp_audio))
	if d.dev == nil {
		return nil, fmt.Errorf("%w: alloc", ErrDeviceInit)
	}
	var playName, capName *C.char
	if opts.Speaker != "" {
		playName = C.CString(opts.Speaker)
		defer C.free(unsafe.Pointer(playName))
	}
	if opts.Microphone != "" {
		capName = C.CString(opts.Microphone)
		defer C.free(unsafe.Pointer(capName))
	}
	if C.rgesp_audio_init_ex(d.dev, cInt(opts.Role), playName, capName) != 0 {
		C.free(unsafe.Pointer(d.dev))
		d.dev = nil
		return nil, fmt.Errorf("%w: init", ErrDeviceInit)
	}
	return d, nil
}

func Backend() string { return "miniaudio" }

func (d *miniaudioDevice) Start() error {
	if d.closed || d.dev == nil {
		return ErrDeviceClosed
	}
	if C.rgesp_audio_start(d.dev) != 0 {
		return fmt.Errorf("miniaudio start failed")
	}
	return nil
}

func (d *miniaudioDevice) ReadPCM() ([]int16, error) {
	if d.closed || d.dev == nil {
		return nil, ErrDeviceClosed
	}
	if cap(d.pcm) < d.frameSize {
		d.pcm = make([]int16, d.frameSize)
	}
	d.pcm = d.pcm[:d.frameSize]
	n := C.rgesp_audio_read(d.dev, (*C.short)(unsafe.Pointer(&d.pcm[0])), cInt(d.frameSize))
	if n <= 0 {
		clear(d.pcm)
		d.watchCapture(d.pcm)
		return d.pcm, nil
	}
	d.watchCapture(d.pcm[:n])
	return d.pcm[:n], nil
}

// watchCapture mirrors upstream LXST 0.6.0 LineSource signal monitoring:
// when the input produces only zero samples for longer than
// SIGNAL_FAIL_REINIT_THRESHOLD the recorder is re-initialised.
func (d *miniaudioDevice) watchCapture(pcm []int16) {
	if !d.checkSignal || d.signalOK {
		return
	}
	for _, s := range pcm {
		if s != 0 {
			d.signalOK = true
			return
		}
	}
	if d.zeroSince.IsZero() {
		d.zeroSince = time.Now()
		return
	}
	if time.Since(d.zeroSince) > signalFailReinitThreshold {
		d.zeroSince = time.Now()
		_ = C.rgesp_audio_reinit(d.dev)
	}
}

func (d *miniaudioDevice) WritePCM(pcm []int16) error {
	if d.closed || d.dev == nil {
		return ErrDeviceClosed
	}
	if !d.duplex || len(pcm) == 0 {
		return nil
	}
	_ = C.rgesp_audio_write(d.dev, (*C.short)(unsafe.Pointer(&pcm[0])), cInt(len(pcm)))
	return nil
}

func (d *miniaudioDevice) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	if d.dev != nil {
		C.rgesp_audio_close(d.dev)
		C.free(unsafe.Pointer(d.dev))
		d.dev = nil
	}
	return nil
}

// #nosec G115 -- audio parameters are bounded before C API calls
func cInt(v int) C.int {
	return C.int(v)
}
