// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// mediaConversionTimeout matches Python CONVERSION_TIMEOUT.
const mediaConversionTimeout = 8 * time.Second

// mediaBackends are the supported WebP encoders in preference order, matching
// Python BACKENDS. RNGIT_MEDIA_BACKEND forces a specific backend.
var mediaBackends = []struct {
	name string
	argv []string
}{
	{"magick", []string{"magick", "-", "webp:-"}},
	{"convert", []string{"convert", "-", "webp:-"}},
	{"gm", []string{"gm", "convert", "-", "webp:-"}},
	{"ffmpeg", []string{"ffmpeg", "-y", "-loglevel", "error", "-i", "-", "-f", "webp", "pipe:1"}},
	{"avconv", []string{"avconv", "-y", "-loglevel", "error", "-i", "-", "-f", "webp", "pipe:1"}},
}

var mediaWinner struct {
	mu   sync.Mutex
	name string
}

// AvailableMediaBackends reports which encoder names resolve on PATH.
func AvailableMediaBackends() map[string]bool {
	out := map[string]bool{}
	for _, b := range mediaBackends {
		_, err := exec.LookPath(b.argv[0])
		out[b.name] = err == nil
	}
	return out
}

// selectedMediaBackend picks the encoder, honoring RNGIT_MEDIA_BACKEND and the
// last successful backend preference, matching Python _selected_backend.
func selectedMediaBackend() (string, []string, bool) {
	if env := os.Getenv("RNGIT_MEDIA_BACKEND"); env != "" {
		for _, b := range mediaBackends {
			if b.name == env {
				if _, err := exec.LookPath(b.argv[0]); err == nil {
					return b.name, b.argv, true
				}
				return "", nil, false
			}
		}
		return "", nil, false
	}
	order := mediaBackends
	mediaWinner.mu.Lock()
	winner := mediaWinner.name
	mediaWinner.mu.Unlock()
	if winner != "" {
		order = make([]struct {
			name string
			argv []string
		}, 0, len(mediaBackends))
		for _, b := range mediaBackends {
			if b.name == winner {
				order = append([]struct {
					name string
					argv []string
				}{b}, order...)
			} else {
				order = append(order, b)
			}
		}
	}
	for _, b := range order {
		if _, err := exec.LookPath(b.argv[0]); err == nil {
			mediaWinner.mu.Lock()
			mediaWinner.name = b.name
			mediaWinner.mu.Unlock()
			return b.name, b.argv, true
		}
	}
	return "", nil, false
}

// configuredMediaArgv applies quality and dimension options to the backend
// argv, matching Python _configured_backend.
func configuredMediaArgv(quality, maxDimension int) (string, []string, bool) {
	name, argv, ok := selectedMediaBackend()
	if !ok {
		return "", nil, false
	}
	argv = append([]string(nil), argv...)
	qualityArg := 0
	if quality > 0 {
		qualityArg = max(1, min(100, quality))
	}
	dimensionArg := maxDimension
	if dimensionArg < 1 {
		dimensionArg = 0
	}
	switch name {
	case "magick", "convert", "gm":
		var options []string
		if qualityArg > 0 {
			options = append(options, "-quality", strconv.Itoa(qualityArg))
		}
		if dimensionArg > 0 {
			d := strconv.Itoa(dimensionArg)
			options = append(options, "-resize", d+"x"+d+">")
		}
		argv = append(argv[:len(argv)-1], append(options, argv[len(argv)-1])...)
	case "ffmpeg", "avconv":
		var options []string
		if qualityArg > 0 {
			options = append(options, "-quality", strconv.Itoa(qualityArg))
		}
		if dimensionArg > 0 {
			d := strconv.Itoa(dimensionArg)
			options = append(options, "-vf", "scale='min(iw,"+d+")':'min(ih,"+d+")':force_original_aspect_ratio=decrease")
		}
		formatIndex := len(argv)
		for i, a := range argv {
			if a == "-f" {
				formatIndex = i
				break
			}
		}
		argv = append(argv[:formatIndex], append(options, argv[formatIndex:]...)...)
	}
	return name, argv, true
}

// webpInfo parses a WebP RIFF header, matching Python _webp_info.
func webpInfo(data []byte) (width, height int, ok bool) {
	if len(data) < 30 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return 0, 0, false
	}
	fourcc := string(data[12:16])
	switch fourcc {
	case "VP8X":
		width = int(data[24]) | int(data[25])<<8 | int(data[26])<<16
		height = int(data[27]) | int(data[28])<<8 | int(data[29])<<16
		width++
		height++
	case "VP8 ":
		width = int(binary.LittleEndian.Uint16(data[26:28])) & 0x3FFF
		height = int(binary.LittleEndian.Uint16(data[28:30])) & 0x3FFF
	case "VP8L":
		bits := binary.LittleEndian.Uint32(data[21:25])
		width = int(bits&0x3FFF) + 1
		height = int((bits>>14)&0x3FFF) + 1
	default:
		return 0, 0, false
	}
	if width > 0 && height > 0 {
		return width, height, true
	}
	return 0, 0, false
}

// validWebP checks whether path contains a valid WebP file.
func validWebP(path string) bool {
	fh, err := os.Open(path) // #nosec G304 -- conversion output
	if err != nil {
		return false
	}
	defer fh.Close()
	buf := make([]byte, 30)
	n, err := fh.Read(buf)
	if err != nil && n == 0 {
		return false
	}
	_, _, ok := webpInfo(buf[:n])
	return ok
}

// ConvertToWebP runs inputArgv piped through the selected WebP encoder into
// outputPath, matching Python media.convert_to_webp.
func ConvertToWebP(inputArgv []string, outputPath, cwd string, timeout time.Duration, quality, maxDimension int) bool {
	backend, encoderArgv, ok := configuredMediaArgv(quality, maxDimension)
	if !ok {
		return false
	}
	if timeout <= 0 {
		timeout = mediaConversionTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out, err := os.Create(outputPath) // #nosec G304 -- conversion output
	if err != nil {
		return false
	}
	defer out.Close()

	inputProc := exec.CommandContext(ctx, inputArgv[0], inputArgv[1:]...) // #nosec G204 -- fixed argv from caller
	inputProc.Dir = cwd
	pipe, err := inputProc.StdoutPipe()
	if err != nil {
		return false
	}
	encoderProc := exec.CommandContext(ctx, encoderArgv[0], encoderArgv[1:]...) // #nosec G204 -- fixed backend argv
	encoderProc.Stdin = pipe
	encoderProc.Stdout = out

	if err := encoderProc.Start(); err != nil {
		return false
	}
	if err := inputProc.Start(); err != nil {
		_ = encoderProc.Wait()
		return false
	}
	encErr := encoderProc.Wait()
	_ = inputProc.Wait()
	if encErr != nil || ctx.Err() != nil {
		_ = backend
		return false
	}
	if !validWebP(outputPath) {
		return false
	}
	return true
}

// ConvertFileToWebP encodes a file to WebP and returns the temporary output
// path, matching Python convert_file_to_webp.
func ConvertFileToWebP(sourcePath string, quality, maxDimension int, timeout time.Duration) (string, bool) {
	_, encoderArgv, ok := configuredMediaArgv(quality, maxDimension)
	if !ok {
		return "", false
	}
	if st, err := os.Stat(sourcePath); err != nil || !st.Mode().IsRegular() {
		return "", false
	}
	if timeout <= 0 {
		timeout = mediaConversionTimeout
	}
	tmp, err := os.CreateTemp("", "rns_media_*.webp")
	if err != nil {
		return "", false
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		tmp.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	in, err := os.Open(sourcePath) // #nosec G304 -- operator media source
	if err != nil {
		return "", false
	}
	defer in.Close()

	cmd := exec.CommandContext(ctx, encoderArgv[0], encoderArgv[1:]...) // #nosec G204 -- fixed backend argv
	cmd.Stdin = in
	cmd.Stdout = tmp
	if err := cmd.Run(); err != nil || ctx.Err() != nil {
		return "", false
	}
	if !validWebP(tmpPath) {
		return "", false
	}
	success = true
	return tmpPath, true
}

// spoolWebP converts a git blob to a temporary WebP file inside dir and reads
// it back, returning data and the response name.
func (n *Node) spoolWebP(repoPath, ref, filePath string) ([]byte, string, bool) {
	tmpdir, err := os.MkdirTemp("", "rnsgit-media-")
	if err != nil {
		return nil, "", false
	}
	defer os.RemoveAll(tmpdir)
	stem := fileStem(filepath.Base(filePath))
	spoolPath := filepath.Join(tmpdir, stem+".webp")
	if !ConvertToWebP([]string{"git", "show", ref + ":" + filePath}, spoolPath, repoPath, 0, 0, 0) {
		return nil, "", false
	}
	data, err := os.ReadFile(spoolPath) // #nosec G304 -- conversion spool
	if err != nil {
		return nil, "", false
	}
	return data, stem + ".webp", true
}

func fileStem(name string) string {
	for i := len(name) - 1; i > 0; i-- {
		if name[i] == '.' {
			return name[:i]
		}
	}
	return name
}
