// SPDX-License-Identifier: LicenseRef-Reticulum
package phonebook

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/sandbox"
)

const (
	maxConfigBytes      = 1 << 20
	maxConfigLines      = 16384
	scanBufInit         = 64 * 1024
	scanBufMax          = 1024 * 1024
	maxPhonebookEntries = 2048
)

// Config is the rnphone-compatible INI subset.
type Config struct {
	Ringtone       string
	Speaker        string
	Microphone     string
	Ringer         string
	AllowedCallers string
	BlockedCallers string
	AutoAnswer     string
	Phonebook      []Entry
	Raw            map[string]map[string]string
}

func LoadINI(path string) (Config, error) {
	if strings.ContainsRune(path, 0) {
		return Config{}, fmt.Errorf("invalid path")
	}
	f, err := os.Open(path) // #nosec G304 -- operator-supplied local phonebook path after NUL check
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > maxConfigBytes {
		return Config{}, fmt.Errorf("%s: config larger than 1MB", path)
	}
	cfg := Config{Raw: map[string]map[string]string{}}
	section := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, scanBufInit), scanBufMax)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if lineNo > maxConfigLines {
			return Config{}, fmt.Errorf("%s: too many lines", path)
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if err := applyINILine(&cfg, &section, line); err != nil {
			return Config{}, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyINILine(cfg *Config, section *string, line string) error {
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		*section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
		if sandbox.ForbiddenConfigKey(*section) {
			return fmt.Errorf("%s cannot configure or disable the sandbox", *section)
		}
		if _, ok := cfg.Raw[*section]; !ok {
			cfg.Raw[*section] = map[string]string{}
		}
		return nil
	}
	key, val, ok := strings.Cut(line, "=")
	if !ok {
		return fmt.Errorf("expected key = value")
	}
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if *section != "phonebook" && sandbox.ForbiddenConfigKey(key) {
		return fmt.Errorf("%s cannot configure or disable the sandbox", key)
	}
	if cfg.Raw[*section] == nil {
		cfg.Raw[*section] = map[string]string{}
	}
	cfg.Raw[*section][key] = val
	return applyINIValue(cfg, *section, key, val)
}

func applyINIValue(cfg *Config, section, key, val string) error {
	switch section {
	case "telephone":
		applyTelephoneKey(cfg, key, val)
	case "phonebook":
		e, err := parsePhonebookValue(key, val)
		if err != nil {
			return err
		}
		cfg.Phonebook = append(cfg.Phonebook, e)
		if len(cfg.Phonebook) > maxPhonebookEntries {
			return fmt.Errorf("too many phonebook entries")
		}
	}
	return nil
}

func applyTelephoneKey(cfg *Config, key, val string) {
	switch strings.ToLower(key) {
	case "ringtone":
		cfg.Ringtone = val
	case "speaker":
		cfg.Speaker = val
	case "microphone":
		cfg.Microphone = val
	case "ringer":
		cfg.Ringer = val
	case "allowed_callers":
		cfg.AllowedCallers = val
	case "blocked_callers":
		cfg.BlockedCallers = val
	case "auto_answer":
		cfg.AutoAnswer = val
	}
}

func parsePhonebookValue(name, val string) (Entry, error) {
	hashPart, alias, _ := strings.Cut(val, ",")
	h, err := ParseHash(strings.TrimSpace(hashPart))
	if err != nil {
		return Entry{}, err
	}
	return Entry{Name: name, Hash: h, Alias: strings.TrimSpace(alias)}, nil
}

func ApplyPolicy(book *Book, cfg Config) error {
	for _, e := range cfg.Phonebook {
		if err := book.Add(e); err != nil {
			return err
		}
	}
	allowed := strings.ToLower(strings.TrimSpace(cfg.AllowedCallers))
	switch allowed {
	case "", "all":
		book.SetPolicy(AllowAll)
	case "none":
		book.SetPolicy(AllowNone)
	case "phonebook":
		book.AllowPhonebook()
	default:
		hashes, err := parseHashList(cfg.AllowedCallers)
		if err != nil {
			return err
		}
		book.SetAllowed(hashes)
	}
	if strings.TrimSpace(cfg.BlockedCallers) != "" {
		hashes, err := parseHashList(cfg.BlockedCallers)
		if err != nil {
			return err
		}
		book.SetBlocked(hashes)
	}
	return nil
}

func parseHashList(val string) ([][]byte, error) {
	parts := strings.FieldsFunc(val, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	var out [][]byte
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || p == "all" || p == "none" || p == "phonebook" {
			continue
		}
		h, err := ParseHash(p)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}
