package interfaces

import (
	"strings"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func TestNewFromConfigUnsupported(t *testing.T) {
	_, err := NewFromConfig("x", &common.InterfaceConfig{Type: "NoSuchInterface", Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestNewFromConfigNil(t *testing.T) {
	_, err := NewFromConfig("x", nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}
