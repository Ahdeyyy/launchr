//go:build windows

package main

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestClipboardListenerLifecycle(t *testing.T) {
	stop, err := watchClipboard(func() {})
	if err != nil {
		t.Fatal(err)
	}
	stop()
	stop()
}

func TestWindowEnumerationReleasesPerCallState(t *testing.T) {
	for i := 0; i < 32; i++ {
		openWindows()
	}
	enumerationState.Lock()
	defer enumerationState.Unlock()
	if len(enumerationState.lists) != 0 {
		t.Fatal("enumeration retained result lists")
	}
}

func TestNativeIconApartment(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		load, closeWorker, err := nativeIconLoader()
		if err != nil {
			done <- err
			return
		}
		defer closeWorker()
		path, err := os.Executable()
		if err != nil {
			done <- err
			return
		}
		img, err := load(path)
		if err == nil && (img == nil || img.Bounds().Dx() != 48 || img.Bounds().Dy() != 48) {
			t.Error("invalid native icon size")
		}
		done <- err
	}()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestClipboardDIBConversion(t *testing.T) {
	data := make([]byte, 44)
	binary.LittleEndian.PutUint32(data, 40)
	binary.LittleEndian.PutUint32(data[4:], 1)
	binary.LittleEndian.PutUint32(data[8:], 1)
	binary.LittleEndian.PutUint16(data[14:], 24)
	data[40], data[41], data[42] = 10, 20, 30
	img, err := decodeClipboardDIB(data)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r != 30*257 || g != 20*257 || b != 10*257 || a != 65535 {
		t.Fatal("DIB pixel channels changed")
	}
	for _, bad := range [][]byte{data[:39], data[:42]} {
		if _, err := decodeClipboardDIB(bad); err == nil {
			t.Fatal("accepted truncated DIB")
		}
	}
	binary.LittleEndian.PutUint32(data[4:], 0x7fffffff)
	if _, err := decodeClipboardDIB(data); err == nil {
		t.Fatal("accepted oversized bitmap")
	}
}
