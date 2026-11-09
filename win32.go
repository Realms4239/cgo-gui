//go:build windows

// Couche Win32 minimale via LazyDLL (x/sys ne wrappe pas user32/gdi32 au
// complet). Noms en français calqués sur l'API pour rester lisible.
package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	pRegisterClassExW = user32.NewProc("RegisterClassExW")
	pCreateWindowExW  = user32.NewProc("CreateWindowExW")
	pDefWindowProcW   = user32.NewProc("DefWindowProcW")
	pShowWindow       = user32.NewProc("ShowWindow")
	pUpdateWindow     = user32.NewProc("UpdateWindow")
	pGetMessageW      = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessageW = user32.NewProc("DispatchMessageW")
	pPostMessageW     = user32.NewProc("PostMessageW")
	pSendMessageW     = user32.NewProc("SendMessageW")
	pSetWindowTextW   = user32.NewProc("SetWindowTextW")
	pGetWindowTextW   = user32.NewProc("GetWindowTextW")
	pGetWindowTextLen = user32.NewProc("GetWindowTextLengthW")
	pEnableWindow     = user32.NewProc("EnableWindow")
	pSetTimer         = user32.NewProc("SetTimer")
	pKillTimer        = user32.NewProc("KillTimer")
	pSetFocus         = user32.NewProc("SetFocus")
	pLoadImageW       = user32.NewProc("LoadImageW")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pGetModuleHandle  = modKernel32.NewProc("GetModuleHandleW")

	pCreateFontIndirectW = gdi32.NewProc("CreateFontIndirectW")
	pDeleteObject        = gdi32.NewProc("DeleteObject")
)

var modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

const (
	wsOverlappedwindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsVscroll          = 0x00200000
	wsBorder           = 0x00800000
	wsTabstop          = 0x00010000
	wsGroup            = 0x00020000

	bsPushbutton    = 0x00000000
	bsDefpushbutton = 0x00000001
	bsGroupbox      = 0x00000007

	lbsNotify           = 0x0001
	lbsNoIntegralHeight = 0x0100

	esMultiline   = 0x0004
	esReadonly    = 0x0800
	esAutovscroll = 0x0040
	esAutohscroll = 0x0080

	ssLeft = 0x0000

	swShowNormal = 1

	wmCreate  = 0x0001
	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmCommand = 0x0111
	wmTimer   = 0x0113
	wmSetfont = 0x0030

	bnClicked = 0
	lbnDblclk = 2

	lbAddstring    = 0x0180
	lbGetcursel    = 0x0188
	lbGettext      = 0x0189
	lbGettextlen   = 0x018A
	lbResetcontent = 0x0184
	lbSetcursel    = 0x0186

	emSetsel       = 0x00B1
	emScrollcaret  = 0x00B7
	emGetlinecount = 0x00BA
	emReplacesel   = 0x00C2

	wmSeticon      = 0x0080
	imageIcon      = 1
	lrLoadfromfile = 0x0010

	wmAppLog    = 0x8001
	wmAppDone   = 0x8002
	wmAppVMs    = 0x8003
	wmAppStatus = 0x8004
	wmAppDone2  = 0x8005
)

type wndClassex struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type winMsg struct {
	hwnd    windows.HWND
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

type logfont struct {
	height         int32
	width          int32
	escapement     int32
	orientation    int32
	weight         int32
	italic         byte
	underline      byte
	strikeOut      byte
	charSet        byte
	outPrecision   byte
	clipPrecision  byte
	quality        byte
	pitchAndFamily byte
	faceName       [32]uint16
}

func utf16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func sendMsg(hwnd windows.HWND, msg uint32, w, l uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(uintptr(hwnd), uintptr(msg), w, l)
	return r
}

func setText(hwnd windows.HWND, s string) {
	pSendMessageW.Call(uintptr(hwnd), 0x000C /* WM_SETTEXT */, 0, uintptr(unsafe.Pointer(utf16(s))))
}

func getText(hwnd windows.HWND) string {
	n, _, _ := pGetWindowTextLen.Call(uintptr(hwnd))
	if n <= 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))
	return windows.UTF16ToString(buf)
}

func postMsg(hwnd windows.HWND, msg uint32) {
	pPostMessageW.Call(uintptr(hwnd), uintptr(msg), 0, 0)
}
