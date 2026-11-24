// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

// SelfCheckOptions configures local validation.
type SelfCheckOptions struct {
	Home         string
	RNSConfigDir string
	UDPListen    string
	UDPForward   string
}

// SelfCheckResult is one self-check step.
type SelfCheckResult struct {
	Name   string
	OK     bool
	Detail string
}

// RunSelfCheck validates golxmd home directories and optional UDP transport.
func RunSelfCheck(opts SelfCheckOptions) []SelfCheckResult {
	home := opts.Home
	if home == "" {
		home = DefaultHome()
	}
	home = expandPath(home)

	cfgPath := filepath.Join(home, "config")
	identPath := filepath.Join(home, "identity")
	storageDir := filepath.Join(home, "storage")
	messagesDir := filepath.Join(storageDir, "messages")

	var out []SelfCheckResult

	out = append(out, checkDirWritable("home directory", home))
	out = append(out, checkRoundTripWrite("storage directory", storageDir))
	out = append(out, checkRoundTripWrite("messages directory", messagesDir))

	created, err := FirstRun(home, cfgPath, identPath, storageDir, messagesDir, rnsConfigDirForSelfCheck(opts, home))
	if err != nil {
		out = append(out, SelfCheckResult{Name: "first run", OK: false, Detail: err.Error()})
	} else if created {
		out = append(out, SelfCheckResult{Name: "first run", OK: true, Detail: "created default config and identity"})
	} else {
		out = append(out, SelfCheckResult{Name: "first run", OK: true, Detail: "existing install"})
	}

	if _, err := os.Stat(cfgPath); err != nil {
		out = append(out, SelfCheckResult{Name: "config file", OK: false, Detail: err.Error()})
	} else {
		cfg, err := lxmf.LoadConfig(cfgPath)
		if err != nil {
			out = append(out, SelfCheckResult{Name: "config file", OK: false, Detail: err.Error()})
		} else {
			out = append(out, SelfCheckResult{
				Name:   "config file",
				OK:     true,
				Detail: fmt.Sprintf("display_name=%q propagation=%v", cfg.LXMF.DisplayName, cfg.Propagation.EnableNode),
			})
		}
	}

	if _, err := os.Stat(identPath); err != nil {
		out = append(out, SelfCheckResult{Name: "identity file", OK: false, Detail: err.Error()})
	} else {
		id, err := identity.FromFile(identPath)
		if err != nil {
			out = append(out, SelfCheckResult{Name: "identity file", OK: false, Detail: err.Error()})
		} else {
			out = append(out, SelfCheckResult{
				Name:   "identity file",
				OK:     true,
				Detail: fmt.Sprintf("hash=%x", id.Hash()),
			})
		}
	}

	rnsDir := rnsConfigDirForSelfCheck(opts, home)
	out = append(out, checkDirReadable("RNS config directory", rnsDir))
	if _, err := os.Stat(filepath.Join(rnsDir, "config")); err != nil {
		out = append(out, SelfCheckResult{Name: "RNS config file", OK: false, Detail: err.Error()})
	} else {
		out = append(out, SelfCheckResult{Name: "RNS config file", OK: true, Detail: filepath.Join(rnsDir, "config")})
	}

	if opts.UDPListen != "" && opts.UDPForward != "" {
		out = append(out, checkUDPTransport(opts.UDPListen, opts.UDPForward))
	}

	return out
}

func rnsConfigDirForSelfCheck(opts SelfCheckOptions, home string) string {
	if strings.TrimSpace(opts.RNSConfigDir) != "" {
		return ResolveRNSConfigDir(opts.RNSConfigDir)
	}
	if opts.Home != "" && expandPath(opts.Home) != expandPath(DefaultHome()) {
		return filepath.Join(home, "rns")
	}
	return ResolveRNSConfigDir("")
}

func checkDirWritable(name, path string) SelfCheckResult {
	if err := ensurePrivateDir(path); err != nil {
		return SelfCheckResult{Name: name, OK: false, Detail: err.Error()}
	}
	testPath := filepath.Join(path, ".golxmd-selfcheck")
	if err := os.WriteFile(testPath, []byte("ok"), 0o600); err != nil {
		return SelfCheckResult{Name: name, OK: false, Detail: "not writable: " + err.Error()}
	}
	_ = os.Remove(testPath)
	return SelfCheckResult{Name: name, OK: true, Detail: path}
}

func checkDirReadable(name, path string) SelfCheckResult {
	if path == "" {
		return SelfCheckResult{Name: name, OK: false, Detail: "path unavailable"}
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SelfCheckResult{Name: name, OK: true, Detail: path + " (will be created by Reticulum)"}
		}
		return SelfCheckResult{Name: name, OK: false, Detail: err.Error()}
	}
	if !info.IsDir() {
		return SelfCheckResult{Name: name, OK: false, Detail: "not a directory"}
	}
	return SelfCheckResult{Name: name, OK: true, Detail: path}
}

func checkRoundTripWrite(name, path string) SelfCheckResult {
	if err := ensurePrivateDir(path); err != nil {
		return SelfCheckResult{Name: name, OK: false, Detail: err.Error()}
	}
	testPath := filepath.Join(path, ".golxmd-write-test")
	payload := []byte("golxmd-selfcheck")
	if err := os.WriteFile(testPath, payload, 0o600); err != nil {
		return SelfCheckResult{Name: name, OK: false, Detail: "write failed: " + err.Error()}
	}
	got, err := os.ReadFile(filepath.Clean(testPath)) // #nosec G304 -- self-check temp file
	_ = os.Remove(testPath)
	if err != nil {
		return SelfCheckResult{Name: name, OK: false, Detail: "read failed: " + err.Error()}
	}
	if string(got) != string(payload) {
		return SelfCheckResult{Name: name, OK: false, Detail: "read/write mismatch"}
	}
	return SelfCheckResult{Name: name, OK: true, Detail: path}
}

func checkUDPTransport(listen, forward string) SelfCheckResult {
	log, closer, err := ConfigureLogging(LogConfig{Level: lxmf.LogWarning, Console: false, File: ""})
	if err != nil {
		return SelfCheckResult{Name: "UDP transport", OK: false, Detail: err.Error()}
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	tr, ifaces, err := startTransport(TransportConfig{
		RNSConfigDir: ResolveRNSConfigDir(""),
		UDPListen:    listen,
		UDPForward:   forward,
	}, log)
	if err != nil {
		return SelfCheckResult{Name: "UDP transport", OK: false, Detail: err.Error()}
	}
	for _, iface := range ifaces {
		_ = iface.Stop()
	}
	_ = tr.Close()
	return SelfCheckResult{
		Name:   "UDP transport",
		OK:     true,
		Detail: fmt.Sprintf("listen=%s forward=%s", udpAddr(listen), udpAddr(forward)),
	}
}

// SelfCheckPassed reports whether every result succeeded.
func SelfCheckPassed(results []SelfCheckResult) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return true
}
