package resource

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNewResourceFromBytes(t *testing.T) {
	data := []byte("hello world")
	r, err := New(data, false)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if r.GetDataSize() != int64(len(data)) {
		t.Errorf("Expected size %d, got %d", len(data), r.GetDataSize())
	}
	if r.GetSegments() != 1 {
		t.Errorf("Expected 1 segment, got %d", r.GetSegments())
	}
}

func TestNewResourceFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	data := []byte("file data")
	err := os.WriteFile(tmpFile, data, 0644)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(tmpFile, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r, err := New(f, false)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if r.GetDataSize() != int64(len(data)) {
		t.Errorf("Expected size %d, got %d", len(data), r.GetDataSize())
	}
}

func TestGetSegmentData(t *testing.T) {
	data := make([]byte, DefaultSegmentSize+100)
	for i := range data {
		data[i] = byte(i % 256)
	}

	r, _ := New(data, false)
	if r.GetSegments() != 2 {
		t.Fatalf("Expected 2 segments, got %d", r.GetSegments())
	}

	seg0, err := r.GetSegmentData(0)
	if err != nil {
		t.Fatalf("GetSegmentData(0) failed: %v", err)
	}
	if !bytes.Equal(seg0, data[:DefaultSegmentSize]) {
		t.Error("Segment 0 data mismatch")
	}

	seg1, err := r.GetSegmentData(1)
	if err != nil {
		t.Fatalf("GetSegmentData(1) failed: %v", err)
	}
	if !bytes.Equal(seg1, data[DefaultSegmentSize:]) {
		t.Error("Segment 1 data mismatch")
	}
}

func TestMarkSegmentComplete(t *testing.T) {
	data := make([]byte, DefaultSegmentSize*2)
	r, _ := New(data, false)

	callbackCalled := false
	r.SetCallback(func(res *Resource) {
		callbackCalled = true
	})

	r.MarkSegmentComplete(0)
	if r.GetProgress() != 0.5 {
		t.Errorf("Expected progress 0.5, got %f", r.GetProgress())
	}
	if r.GetStatus() != StatusPending && r.GetStatus() != StatusActive {
		t.Errorf("Wrong status: %v", r.GetStatus())
	}

	r.MarkSegmentComplete(1)
	if r.GetProgress() != 1.0 {
		t.Errorf("Expected progress 1.0, got %f", r.GetProgress())
	}
	if r.GetStatus() != StatusComplete {
		t.Errorf("Expected status COMPLETE, got %v", r.GetStatus())
	}
	if !callbackCalled {
		t.Error("Callback was not called")
	}
}

func TestRead(t *testing.T) {
	data := []byte("hello world")
	r, _ := New(data, false)

	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 5 || !bytes.Equal(buf, []byte("hello")) {
		t.Errorf("Read wrong data: %q", buf)
	}

	buf = make([]byte, 10)
	n, err = r.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 6 || !bytes.Equal(buf[:n], []byte(" world")) {
		t.Errorf("Read wrong data: %q", buf[:n])
	}

	_, err = r.Read(buf)
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestCancelActivateFailed(t *testing.T) {
	data := []byte("test")
	r, _ := New(data, false)

	r.Activate()
	if r.GetStatus() != StatusActive {
		t.Errorf("Expected ACTIVE, got %v", r.GetStatus())
	}

	r.SetFailed()
	if r.GetStatus() != StatusFailed {
		t.Errorf("Expected FAILED, got %v", r.GetStatus())
	}

	r2, _ := New(data, false)
	r2.Cancel()
	if r2.GetStatus() != StatusCancelled {
		t.Errorf("Expected CANCELLED, got %v", r2.GetStatus())
	}
}
