//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")

func winCall(name string, args ...uintptr) uintptr {
	r, _, _ := user32.NewProc(name).Call(args...)
	return r
}
func hideCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

type appWindow struct {
	Handle      uintptr
	Title, Path string
}

func openWindows() []appWindow {
	var result []appWindow
	cb := syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		if winCall("IsWindowVisible", hwnd) == 0 || winCall("GetWindow", hwnd, 4) != 0 {
			return 1
		}
		if winCall("GetWindowLongW", hwnd, uintptr(0xffff_ffec))&0x80 != 0 {
			return 1
		}
		var pid uint32
		winCall("GetWindowThreadProcessId", hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == uint32(os.Getpid()) {
			return 1
		}
		n := winCall("GetWindowTextLengthW", hwnd)
		if n == 0 {
			return 1
		}
		buf := make([]uint16, n+1)
		winCall("GetWindowTextW", hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
		// Cloaked windows belong to other virtual desktops or suspended apps.
		var cloaked uint32
		syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute").Call(hwnd, 14, uintptr(unsafe.Pointer(&cloaked)), 4)
		if cloaked != 0 {
			return 1
		}
		path := ""
		p, _, _ := kernel32.NewProc("OpenProcess").Call(0x1000, 0, uintptr(pid))
		if p != 0 {
			var size uint32 = 32768
			exe := make([]uint16, size)
			r, _, _ := kernel32.NewProc("QueryFullProcessImageNameW").Call(p, 0, uintptr(unsafe.Pointer(&exe[0])), uintptr(unsafe.Pointer(&size)))
			if r != 0 {
				path = syscall.UTF16ToString(exe[:size])
			}
			syscall.CloseHandle(syscall.Handle(p))
		}
		result = append(result, appWindow{hwnd, syscall.UTF16ToString(buf), path})
		return 1
	})
	winCall("EnumWindows", cb, 0)
	return result
}
func activateWindow(w appWindow) error {
	if winCall("IsWindow", w.Handle) == 0 {
		return fmt.Errorf("this window has closed; reopen Launchr to refresh")
	}
	if winCall("IsIconic", w.Handle) != 0 {
		winCall("ShowWindow", w.Handle, 9)
	}
	if winCall("SetForegroundWindow", w.Handle) == 0 {
		return fmt.Errorf("Windows could not focus this app")
	}
	return nil
}

// Clipboard updates use a message-only native window. No timer or polling runs
// while the launcher is hidden. Callbacks only signal work on MyGo's UI thread.
func watchClipboard(notify func()) (func(), error) {
	type wndclass struct {
		Size, Style                        uint32
		Proc                               uintptr
		ClsExtra, WndExtra                 int32
		Instance, Icon, Cursor, Background uintptr
		Menu, Name                         *uint16
		SmallIcon                          uintptr
	}
	type message struct {
		Hwnd           uintptr
		Message        uint32
		WParam, LParam uintptr
		Time           uint32
		X, Y           int32
		Private        uint32
	}
	type ready struct {
		hwnd uintptr
		err  error
	}
	ch := make(chan ready, 1)
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		name, _ := syscall.UTF16PtrFromString("LaunchrClipboardListener")
		inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
		proc := syscall.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
			switch msg {
			case 0x031d:
				notify()
				return 0
			case 0x0010:
				winCall("RemoveClipboardFormatListener", hwnd)
				winCall("DestroyWindow", hwnd)
				return 0
			case 0x0002:
				winCall("PostQuitMessage", 0)
				return 0
			}
			return winCall("DefWindowProcW", hwnd, uintptr(msg), wp, lp)
		})
		wc := wndclass{Proc: proc, Instance: inst, Name: name}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if winCall("RegisterClassExW", uintptr(unsafe.Pointer(&wc))) == 0 {
			ch <- ready{err: fmt.Errorf("register clipboard listener failed")}
			return
		}
		defer winCall("UnregisterClassW", uintptr(unsafe.Pointer(name)), inst)
		hwnd := winCall("CreateWindowExW", 0, uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, inst, 0)
		if hwnd == 0 {
			ch <- ready{err: fmt.Errorf("create clipboard listener failed")}
			return
		}
		if winCall("AddClipboardFormatListener", hwnd) == 0 {
			winCall("DestroyWindow", hwnd)
			ch <- ready{err: fmt.Errorf("clipboard listening unavailable")}
			return
		}
		ch <- ready{hwnd: hwnd}
		var msg message
		for {
			r := winCall("GetMessageW", uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if r == 0 || int32(r) == -1 {
				break
			}
			winCall("TranslateMessage", uintptr(unsafe.Pointer(&msg)))
			winCall("DispatchMessageW", uintptr(unsafe.Pointer(&msg)))
		}
	}()
	r := <-ch
	if r.err != nil {
		return nil, r.err
	}
	var once sync.Once
	return func() { once.Do(func() { winCall("PostMessageW", r.hwnd, 0x10, 0, 0); <-done }) }, nil
}
