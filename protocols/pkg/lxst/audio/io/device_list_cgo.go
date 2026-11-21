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

#include "miniaudio.h"
#include <string.h>

typedef struct {
	char name[256];
	int capture;
} rgesp_listed_dev;

static int rgesp_list_devices(rgesp_listed_dev *out, int maxn) {
	ma_context ctx;
	ma_device_info *play = NULL;
	ma_device_info *cap = NULL;
	ma_uint32 nplay = 0;
	ma_uint32 ncap = 0;
	int n = 0;
	ma_uint32 i;
	if (maxn <= 0 || out == NULL) {
		return 0;
	}
	if (ma_context_init(NULL, 0, NULL, &ctx) != MA_SUCCESS) {
		return -1;
	}
	if (ma_context_get_devices(&ctx, &play, &nplay, &cap, &ncap) != MA_SUCCESS) {
		ma_context_uninit(&ctx);
		return -1;
	}
	for (i = 0; i < nplay && n < maxn; i++) {
		memset(&out[n], 0, sizeof(rgesp_listed_dev));
		strncpy(out[n].name, play[i].name, 255);
		out[n].capture = 0;
		n++;
	}
	for (i = 0; i < ncap && n < maxn; i++) {
		memset(&out[n], 0, sizeof(rgesp_listed_dev));
		strncpy(out[n].name, cap[i].name, 255);
		out[n].capture = 1;
		n++;
	}
	ma_context_uninit(&ctx);
	return n;
}
*/
import "C"

type DeviceInfo struct {
	Name    string
	Capture bool
}

func ListDevices() ([]DeviceInfo, error) {
	const maxn = 64
	buf := make([]C.rgesp_listed_dev, maxn)
	n := int(C.rgesp_list_devices(&buf[0], C.int(maxn)))
	if n < 0 {
		return nil, nil
	}
	out := make([]DeviceInfo, 0, n)
	for i := 0; i < n; i++ {
		name := C.GoString(&buf[i].name[0])
		out = append(out, DeviceInfo{Name: name, Capture: buf[i].capture != 0})
	}
	return out, nil
}
