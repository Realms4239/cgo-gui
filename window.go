//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32W = windows.NewLazySystemDLL("user32.dll")
	pMsgBox    = modUser32W.NewProc("MessageBoxW")
)

func (a *app) run() error {
	// VITAL : la pompe GetMessage + toutes les fenêtres DOIVENT vivre sur
	// le MÊME thread OS (contrat Win32 : file de messages par thread).
	// Sans ça, Go migre la goroutine, les PostMessage arrivent sur l'ancien
	// thread que plus personne ne pompe (freeze total, CPU idle) ou
	// GetMessage échoue et l'app s'éteint en silence. C'ÉTAIT le freeze.
	runtime.LockOSThread()
	className := utf16("MeteolinkKitWnd")
	var wc wndClassex
	wc.size = uint32(unsafe.Sizeof(wc))
	wc.wndProc = wndProcPtr
	var hMod windows.Handle
	_, _, _ = pGetModuleHandle.Call(0, 0, uintptr(unsafe.Pointer(&hMod)))
	wc.instance = hMod
	wc.background = windows.Handle(16) // COLOR_BTNFACE+1
	wc.className = className
	if _, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); err != nil {
		if errno, ok := err.(windows.Errno); !ok || errno != 0 {
			return fmt.Errorf("classe fenêtre : %v", err)
		}
	}
	title := utf16(appTitle)
	hwnd, _, err := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsOverlappedwindow&^0x00010000&^0x00020000), // fixe : ni minimize ni maximize
		100, 100, uintptr(winW), uintptr(winH),
		0, 0, uintptr(wc.instance), 0)
	if hwnd == 0 {
		return fmt.Errorf("fenêtre : %v", err)
	}
	a.hwnd = windows.HWND(hwnd)
	a.hfont = mkFont()
	a.buildControls()
	a.setIcon()
	pShowWindow.Call(uintptr(a.hwnd), swShowNormal)
	pUpdateWindow.Call(uintptr(a.hwnd))
	// ticker 200 ms : flush du journal (5 img/s max) + balayage lent.
	// SANS ce timer, rien ne rafraîchit : le handler wmTimer existait seul.
	pSetTimer.Call(uintptr(a.hwnd), 1, 200, 0)
	// état initial : scan léger en fond
	go a.refreshVMs()
	go a.refreshStatus()
	var msg winMsg
	for {
		ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return nil
}

func (a *app) setIcon() {
	// D'abord l'icône COMPILÉE dans l'exe (ressource "APP" du .syso) :
	// le logo voyage avec le binaire, plus de dépendance au logo.ico posé.
	if hMod, _, _ := pGetModuleHandle.Call(0, 0, 0); hMod != 0 {
		if h, _, _ := pLoadImageW.Call(hMod, uintptr(unsafe.Pointer(utf16("APP"))), imageIcon, 48, 48, 0); h != 0 {
			sendMsg(a.hwnd, wmSeticon, 0, h)
			sendMsg(a.hwnd, wmSeticon, 1, h)
			return
		}
	}
	// Repli : logo.ico posé à côté (zips historiques).
	exe, err := os.Executable()
	if err != nil {
		return
	}
	ico := filepath.Join(filepath.Dir(exe), "logo.ico")
	h, _, _ := pLoadImageW.Call(0, uintptr(unsafe.Pointer(utf16(ico))), imageIcon, 48, 48, lrLoadfromfile)
	if h != 0 {
		sendMsg(a.hwnd, wmSeticon, 0, h)
		sendMsg(a.hwnd, wmSeticon, 1, h)
	}
}

func wndProc(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	switch msg {
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	case wmClose:
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmAppLog:
		a.drainPending()
		return 0
	case wmAppDone:
		a.onDone()
		return 0
	case wmAppVMs:
		a.onVMs()
		return 0
	case wmAppStatus:
		a.onStatus()
		return 0
	case wmAppDone2:
		a.onScanDone()
		return 0
	case wmTimer:
		a.flushLog()
		a.tick++
		// balayage lent toutes les ~30 s seulement : un SSH toutes les
		// 200 ms serait une tempête (5 connexions/s vers la VM).
		a.mu.Lock()
		idle := a.busy == ""
		a.mu.Unlock()
		if a != nil && idle && a.tick%150 == 0 {
			go a.refreshStatus()
		}
		return 0
	case wmCommand:
		id := int(wParam & 0xFFFF)
		code := int((wParam >> 16) & 0xFFFF)
		// Double-clic liste = verrouiller, DIRECTEMENT (même handler que
		// le bouton Verrouiller) : l'ancien détour par onButton(101)
		// tombait dans le vide (101 n'a pas de verbe buttonAction) et le
		// double-clic passait pour mort (#13).
		if code == lbnDblclk && id == idVMList {
			a.lockSelected()
			return 0
		}
		if code == bnClicked {
			a.onButton(id)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func (a *app) mkCtl(className string, text string, style uint32, x, y, w, h int, id int) windows.HWND {
	hwnd, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(utf16(className))),
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(style|wsChild|wsVisible),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(a.hwnd), uintptr(id), 0, 0)
	if hwnd != 0 && a.hfont != 0 {
		sendMsg(windows.HWND(hwnd), wmSetfont, a.hfont, 1)
	}
	return windows.HWND(hwnd)
}

func mkFont() uintptr {
	var lf logfont
	lf.height = -16
	lf.weight = 400
	name := windows.StringToUTF16("Segoe UI")
	copy(lf.faceName[:], name)
	h, _, _ := pCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return h
}

func (a *app) buildControls() {
	y := 12
	a.mkCtl("STATIC", "Machine virtuelle (VirtualBox / VMware) :", 0, 16, y, 400, 18, 0)
	y += 22
	a.vmList = a.mkCtl("LISTBOX", "", lbsNotify|lbsNoIntegralHeight|wsVscroll|wsBorder|wsTabstop, 16, y, 560, 110, idVMList)
	y += 118
	bx := 16
	for _, b := range groupActions {
		a.btns = append(a.btns, a.mkCtl("BUTTON", b.label, bsPushbutton|wsTabstop, bx, y, b.w, 28, b.id))
		bx += b.w + 8
	}
	y += 40
	a.locked = a.mkCtl("STATIC", "Aucune machine verrouillée.", 0, 16, y, 560, 18, idLocked)
	y += 30
	a.mkCtl("BUTTON", "Accès SSH — diagnostiquer, clé, boot", bsGroupbox, 8, y, 584, 118, 0)
	yy := y + 22
	a.sshLbl = a.mkCtl("STATIC", "SSH : —", 0, 20, yy, 150, 18, idSSH)
	a.mkCtl("STATIC", "Utilisateur Ubuntu :", 0, 180, yy, 125, 18, 0)
	a.userEdit = a.mkCtl("EDIT", "", esAutohscroll|wsBorder|wsTabstop, 310, yy, 140, 22, idUser)
	a.btns = append(a.btns, a.mkCtl("BUTTON", "Sauver", bsPushbutton|wsTabstop, 458, yy-3, 80, 26, 248))
	if u := cfgSSHUser(a.cfgPath); u != "" {
		setText(a.userEdit, u)
	}
	yy += 22
	bx = 20
	for _, b := range accessActions {
		a.btns = append(a.btns, a.mkCtl("BUTTON", b.label, bsPushbutton|wsTabstop, bx, yy, b.w, 28, b.id))
		bx += b.w + 8
	}
	y += 128
	a.mkCtl("BUTTON", "Déployer", bsGroupbox, 8, y, 584, 84, 0)
	yy = y + 24
	a.dashLbl = a.mkCtl("STATIC", "Dashboard : —", 0, 20, yy, 560, 18, idDash)
	yy += 22
	bx = 20
	for _, b := range deployActions {
		a.btns = append(a.btns, a.mkCtl("BUTTON", b.label, bsPushbutton|wsTabstop, bx, yy, b.w, 30, b.id))
		bx += b.w + 8
	}
	// colonne droite : contrôle
	a.mkCtl("BUTTON", "Contrôle", bsGroupbox, 600, 12, 312, 420, 0)
	byID := map[int]buttonDef{}
	for _, b := range controlActions {
		byID[b.id] = b
	}
	cy := 36
	for _, row := range [][]int{{231, 232, 233, 234}, {235, 236, 237, 238}, {239, 240}, {241, 242}, {243, 244}, {245, 246, 247}, {249, 250, 251}} {
		bx := 612
		for _, id := range row {
			b := byID[id]
			a.btns = append(a.btns, a.mkCtl("BUTTON", b.label, bsPushbutton|wsTabstop, bx, cy, b.w, 28, b.id))
			bx += b.w + 8
		}
		cy += 36
	}
	y += 96
	a.mkCtl("BUTTON", "Journal", bsGroupbox, 8, y, 896, 172, 0)
	a.logEdit = a.mkCtl("EDIT", "", esMultiline|esReadonly|esAutovscroll|wsVscroll|wsBorder|wsTabstop, 16, y+24, 880, 112, idLog)
	a.status = a.mkCtl("STATIC", "Prêt.", 0, 16, y+142, 880, 18, idStatus)
	a.nextLbl = a.mkCtl("STATIC", "→ Prochaine : …", 0, 16, y+160, 880, 18, idNext)
}
