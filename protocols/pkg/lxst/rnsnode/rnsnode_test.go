// SPDX-License-Identifier: LicenseRef-Reticulum
package rnsnode_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/rnsnode"
)

func TestStartUDP(t *testing.T) {
	port := freeUDP(t)
	sess, err := rnsnode.Start(rnsnode.Options{
		Kind:       rnsnode.KindUDP,
		ListenPort: port,
		TargetPort: port,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Transport == nil {
		t.Fatal("missing transport")
	}
}

func TestLoadReticulumFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	body := "[reticulum]\nenable_transport = Yes\nshare_instance = No\n\n[interfaces]\n  [[UDP]]\n    type = UDPInterface\n    enabled = yes\n    listen_ip = 127.0.0.1\n    listen_port = 4242\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := rnsnode.LoadReticulumFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EnableTransport || cfg.ShareInstance {
		t.Fatalf("reticulum %+v", cfg)
	}
	ic := cfg.Interfaces["UDP"]
	if ic == nil || ic.Type != "UDPInterface" || ic.Port != 4242 {
		t.Fatalf("iface %+v", ic)
	}
}

func TestLoadReticulumFilePythonAliases(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	body := "[reticulum]\nenable_transport = Yes\ninstance_name = phone\ninstance_control_port = 37429\n\n[interfaces]\n  [[UDP]]\n    type = UDPInterface\n    interface_enabled = No\n    listen_ip = 127.0.0.1\n    listen_port = 4242\n    forward_ip = 10.0.0.2\n    forward_port = 4243\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := rnsnode.LoadReticulumFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InstanceName != "phone" || cfg.InstanceControlPort != 37429 {
		t.Fatalf("reticulum %+v", cfg)
	}
	ic := cfg.Interfaces["UDP"]
	if ic == nil || ic.Enabled || ic.TargetHost != "10.0.0.2" || ic.TargetPort != 4243 {
		t.Fatalf("iface %+v", ic)
	}
}

func TestLoadReticulumFileRejectsHuge(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1<<20 + 8); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := rnsnode.LoadReticulumFile(path); err == nil {
		t.Fatal("expected size rejection")
	}
}

func TestLoadReticulumFileRejectsSandbox(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	body := "[reticulum]\nsandbox = No\nenable_transport = Yes\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := rnsnode.LoadReticulumFile(path); err == nil {
		t.Fatal("sandbox key")
	}
}

func TestLoadReticulumDirLenientIgnoresSandbox(t *testing.T) {
	dir := t.TempDir()
	body := `[reticulum]
  enable_sandbox = yes
  enable_seccomp = yes

[[Test TCP]]
  type = TCPClientInterface
  enabled = yes
  target_host = 127.0.0.1
  target_port = 4242
`
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := rnsnode.LoadReticulumDirLenient(dir)
	if err != nil {
		t.Fatal(err)
	}
	ic := cfg.Interfaces["Test TCP"]
	if ic == nil || !ic.Enabled || ic.TargetHost != "127.0.0.1" || ic.TargetPort != 4242 {
		t.Fatalf("interface=%+v", ic)
	}
}

func TestDefaultConfigDir(t *testing.T) {
	dir := rnsnode.DefaultConfigDir()
	if dir == "" || !strings.HasSuffix(dir, ".reticulum-go") {
		t.Fatalf("dir %q", dir)
	}
}

func TestEnsureDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	created, err := rnsnode.EnsureDefaultConfig(dir)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	cfg, err := rnsnode.LoadOfficialDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	ic := cfg.Interfaces["Auto Discovery"]
	if ic == nil || ic.Type != "AutoInterface" || !ic.Enabled {
		t.Fatalf("interface=%+v", ic)
	}
	created2, err := rnsnode.EnsureDefaultConfig(dir)
	if err != nil || created2 {
		t.Fatalf("second created=%v err=%v", created2, err)
	}
}

func TestStartUnknownKind(t *testing.T) {
	if _, err := rnsnode.Start(rnsnode.Options{Kind: "serial"}); err == nil {
		t.Fatal("expected error")
	}
}

type stubPath struct {
	requested int
}

func (s *stubPath) HasPath([]byte) bool { return false }

func (s *stubPath) RequestPath([]byte, string, []byte, bool) error {
	s.requested++
	return nil
}

func TestWaitRecallFindsSecondHash(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest := append([]byte(nil), id.Hash()...)
	identity.Remember(nil, dest, id.GetPublicKey(), nil)
	miss := bytes.Repeat([]byte{0x11}, 16)
	stub := &stubPath{}
	got, err := rnsnode.WaitRecall(stub, [][]byte{miss, dest}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Hash(), id.Hash()) {
		t.Fatalf("got %x want %x", got.Hash(), id.Hash())
	}
	if stub.requested != 2 {
		t.Fatalf("requested %d", stub.requested)
	}
}

func TestWaitRecallTimeout(t *testing.T) {
	miss := bytes.Repeat([]byte{0x22}, 16)
	if _, err := rnsnode.WaitRecall(&stubPath{}, [][]byte{miss}, 50*time.Millisecond); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestWaitRecallContextCanceled(t *testing.T) {
	miss := bytes.Repeat([]byte{0x33}, 16)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := rnsnode.WaitRecallContext(ctx, &stubPath{}, [][]byte{miss}, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestStartLocalOffline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	_, err = rnsnode.Start(rnsnode.Options{Kind: rnsnode.KindLocal, LocalPort: port})
	if err == nil {
		t.Fatal("expected offline shared instance")
	}
}

func freeUDP(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return port
}
