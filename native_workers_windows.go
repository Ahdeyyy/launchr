//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxCaptureBytes = 64 << 20

// Only this worker touches its COM apartment and the shell objects it creates.
func nativeIconLoader() (func(string) (image.Image, error), func(), error) {
	runtime.LockOSThread()
	ole := syscall.NewLazyDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		runtime.UnlockOSThread()
		return nil, nil, fmt.Errorf("initialize icon apartment: %x", hr)
	}
	return loadNativeIcon, func() { ole.NewProc("CoUninitialize").Call(); runtime.UnlockOSThread() }, nil
}

func comMethod(object uintptr, index int, args ...uintptr) uintptr {
	vtable := *(*uintptr)(nativePointer(object))
	method := *(*uintptr)(nativePointer(vtable + uintptr(index)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(method, append([]uintptr{object}, args...)...)
	return r
}

// Native addresses refer to Windows-owned memory, never Go heap objects.
func nativePointer(address uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&address))
}

func loadNativeIcon(path string) (image.Image, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	iid, _ := windows.GUIDFromString("{bcc18b79-ba16-442f-80c4-8a59c30c463b}")
	var factory uintptr
	hr, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHCreateItemFromParsingName").Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory)))
	if int32(hr) < 0 || factory == 0 {
		return nil, fmt.Errorf("icon lookup: %x", hr)
	}
	defer comMethod(factory, 2)
	var bitmap uintptr
	if hr = comMethod(factory, 3, uintptr(48)|(uintptr(48)<<32), 4, uintptr(unsafe.Pointer(&bitmap))); int32(hr) < 0 || bitmap == 0 {
		return nil, fmt.Errorf("icon extraction: %x", hr)
	}
	gdi := syscall.NewLazyDLL("gdi32.dll")
	defer gdi.NewProc("DeleteObject").Call(bitmap)
	// DIBSECTION layout on Windows/amd64 and arm64.
	var ds struct {
		Type, Width, Height, Stride int32
		Planes, Bits                uint16
		Pixels                      uintptr
		HeaderSize                  uint32
		DIBWidth, DIBHeight         int32
		Rest                        [7]uint32
		Masks                       [3]uint32
		Section                     uintptr
		Offset                      uint32
	}
	r, _, _ := gdi.NewProc("GetObjectW").Call(bitmap, unsafe.Sizeof(ds), uintptr(unsafe.Pointer(&ds)))
	w, h, stride := int(ds.Width), int(ds.Height), int(ds.Stride)
	if r == 0 || ds.Pixels == 0 || ds.Bits != 32 || w <= 0 || h <= 0 || w > 512 || h > 512 || stride < w*4 || stride > 4096 {
		return nil, fmt.Errorf("invalid icon bitmap")
	}
	src := unsafe.Slice((*byte)(nativePointer(ds.Pixels)), stride*h)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := y
		if ds.DIBHeight > 0 {
			sy = h - 1 - y
		}
		for x := 0; x < w; x++ {
			s, d := src[sy*stride+x*4:], dst.Pix[y*dst.Stride+x*4:]
			d[0], d[1], d[2], d[3] = s[2], s[1], s[0], s[3]
		}
	}
	return dst, nil
}

// Snapshot on a dedicated thread, never through MyGo's UI-thread dispatcher.
// Conversion and persistence happen after CloseClipboard has released the lock.
func nativeClipboardSequence() uint32 { return uint32(winCall("GetClipboardSequenceNumber")) }

func readNativeClipboard() (capture, uint32, error) {
	seq := uint32(winCall("GetClipboardSequenceNumber"))
	if winCall("OpenClipboard", 0) == 0 {
		return capture{}, seq, fmt.Errorf("clipboard is busy")
	}
	defer winCall("CloseClipboard")
	pngName, _ := syscall.UTF16PtrFromString("PNG")
	pngFormat := winCall("RegisterClipboardFormatW", uintptr(unsafe.Pointer(pngName)))
	for _, f := range []struct {
		id   uintptr
		kind string
	}{{pngFormat, "image"}, {17, "dib"}, {8, "dib"}, {13, "utf16"}} {
		if f.id == 0 || winCall("IsClipboardFormatAvailable", f.id) == 0 {
			continue
		}
		h := winCall("GetClipboardData", f.id)
		if h == 0 {
			continue
		}
		n, _, _ := kernel32.NewProc("GlobalSize").Call(h)
		if n > maxCaptureBytes {
			return capture{}, seq, fmt.Errorf("clipboard item exceeds 64 MiB capture budget")
		}
		if n == 0 {
			continue
		}
		p, _, _ := kernel32.NewProc("GlobalLock").Call(h)
		if p == 0 {
			continue
		}
		data := bytes.Clone(unsafe.Slice((*byte)(nativePointer(p)), int(n)))
		kernel32.NewProc("GlobalUnlock").Call(h)
		return capture{kind: f.kind, data: data}, uint32(winCall("GetClipboardSequenceNumber")), nil
	}
	return capture{}, seq, nil
}

func normalizeCapture(c capture) (capture, error) {
	switch c.kind {
	case "utf16":
		u := make([]uint16, 0, len(c.data)/2)
		for i := 0; i+1 < len(c.data); i += 2 {
			v := binary.LittleEndian.Uint16(c.data[i:])
			if v == 0 {
				break
			}
			u = append(u, v)
		}
		return capture{kind: "text", data: []byte(string(utf16.Decode(u)))}, nil
	case "dib":
		img, err := decodeClipboardDIB(c.data)
		if err != nil {
			return capture{}, err
		}
		var out bytes.Buffer
		err = png.Encode(&out, img)
		return capture{kind: "image", data: out.Bytes()}, err
	}
	return c, nil
}

func decodeClipboardDIB(data []byte) (image.Image, error) {
	bad := fmt.Errorf("unsupported clipboard bitmap")
	if len(data) < 40 {
		return nil, bad
	}
	le := binary.LittleEndian
	offset := int(le.Uint32(data))
	w, sh := int(int32(le.Uint32(data[4:]))), int(int32(le.Uint32(data[8:])))
	bits, compression := int(le.Uint16(data[14:])), le.Uint32(data[16:])
	h := sh
	if h < 0 {
		h = -h
	}
	if offset < 40 || offset > len(data) || w <= 0 || h <= 0 || w > maxCaptureBytes/4 || h > maxCaptureBytes/(4*w) || (bits != 24 && bits != 32) || (compression != 0 && compression != 3) {
		return nil, bad
	}
	if compression == 3 {
		maskOffset := 40
		if offset == 40 {
			offset += 12
		}
		if len(data) < maskOffset+12 || le.Uint32(data[maskOffset:]) != 0xff0000 || le.Uint32(data[maskOffset+4:]) != 0xff00 || le.Uint32(data[maskOffset+8:]) != 0xff {
			return nil, bad
		}
	}
	stride := ((w*bits + 31) / 32) * 4
	if offset > len(data) || h > (len(data)-offset)/stride {
		return nil, bad
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := y
		if sh > 0 {
			sy = h - 1 - y
		}
		for x := 0; x < w; x++ {
			s, d := data[offset+sy*stride+x*(bits/8):], dst.Pix[y*dst.Stride+x*4:]
			d[0], d[1], d[2], d[3] = s[2], s[1], s[0], 255
			if bits == 32 && le.Uint32(data) >= 108 && le.Uint32(data[52:]) == 0xff000000 {
				d[3] = s[3]
			}
		}
	}
	return dst, nil
}
