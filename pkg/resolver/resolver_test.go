package resolver

import (
	"testing"
)

func TestNew(t *testing.T) {
	r := New()
	if r == nil {
		t.Fatal("New() returned nil")
	}
	if r.cache == nil {
		t.Error("New() cache map is nil")
	}
}

func TestResolver_ResolveIdentity(t *testing.T) {
	r := New()

	tests := []struct {
		name      string
		fullName  string
		wantError bool
	}{
		{
			name:      "ValidName",
			fullName:  "app.aspect",
			wantError: false,
		},
		{
			name:      "EmptyName",
			fullName:  "",
			wantError: true,
		},
		{
			name:      "InvalidFormat",
			fullName:  "app",
			wantError: true,
		},
		{
			name:      "MultiPartName",
			fullName:  "app.aspect1.aspect2",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := r.ResolveIdentity(tt.fullName)
			if (err != nil) != tt.wantError {
				t.Errorf("ResolveIdentity() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError && id == nil {
				t.Error("ResolveIdentity() returned nil identity for valid name")
			}
		})
	}
}

func TestResolver_ResolveIdentity_Caching(t *testing.T) {
	r := New()

	fullName := "app.aspect"
	id1, err := r.ResolveIdentity(fullName)
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}

	id2, err := r.ResolveIdentity(fullName)
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}

	if id1 == nil || id2 == nil {
		t.Fatal("ResolveIdentity() returned nil")
	}

	if id1.GetPublicKey() == nil || id2.GetPublicKey() == nil {
		t.Fatal("Identity public key is nil")
	}

	if string(id1.GetPublicKey()) != string(id2.GetPublicKey()) {
		t.Error("ResolveIdentity() should return cached identity")
	}
}

func TestResolveIdentity(t *testing.T) {
	id, err := ResolveIdentity("app.aspect")
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if id == nil {
		t.Error("ResolveIdentity() returned nil")
	}
}

func TestResolver_ResolveIdentity_Concurrent(t *testing.T) {
	r := New()

	done := make(chan bool, 10)
	for range 10 {
		go func() {
			id, err := r.ResolveIdentity("app.aspect")
			if err != nil {
				t.Errorf("ResolveIdentity() error = %v", err)
			}
			if id == nil {
				t.Error("ResolveIdentity() returned nil")
			}
			done <- true
		}()
	}

	for range 10 {
		<-done
	}
}
