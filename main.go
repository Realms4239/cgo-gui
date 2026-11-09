//go:build windows

// Meteolink Kit — mini application Windows native. Fine coquille autour du
// CLI éprouvé : chaque bouton exécute `cgo.exe kit …` à côté d'elle et
// stream la sortie dans le journal. Les actions interactives (mot de
// passe, admin) s'ouvrent dans une console visible ; le reste tourne en
// fond, résultats via PostMessage (jamais de touché UI hors thread).
package main

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Realms4239/cgo-gui/kit"
	"golang.org/x/sys/windows"
)

const (
	appTitle = "Meteolink Kit — centre de contrôle"
	winW     = 940
	winH     = 700

	idVMList = 101
	idLog    = 102
	idStatus = 103
	idLocked = 104
	idSSH    = 105
	idDash   = 106
	idUser   = 107
	idNext   = 108
)

type app struct {
	hwnd        windows.HWND
	hfont       uintptr
	vmList      windows.HWND
	logEdit     windows.HWND
	status      windows.HWND
	locked      windows.HWND
	sshLbl      windows.HWND
	dashLbl     windows.HWND
	nextLbl     windows.HWND
	userEdit    windows.HWND
	cgoExe      string
	cfgPath     string
	mu          sync.Mutex
	busy        string
	btns        []windows.HWND // boutons d'action (grisés pendant busy)
	pending     []string
	logText     []string
	logDirty    bool
	shown       int // lignes déjà rendues (flush append-only)
	tick        uint64
	statFlight  atomic.Bool // une sonde SSH à la fois
	cfg         cfgCache    // IP invitée connue (fond) — évite vmrun sur le thread UI
	vmRows      []vmRow
	stSSH       string
	stDash      string
	stLocked    string
	stNext      string
	stNextKind  string
	stNextArgs  []string
	stNextDone  bool
	lastNext    string // dernier bandeau journalisé (transitions seules)
	lastSSH     string // dernier état SSH journalisé (transitions seules)
	lastDash    string // dernier état dashboard journalisé (transitions seules)
	chain       bool   // chaîne Suite en cours (avance seule jusqu'au blocage)
	chainVerb   string // dernier verbe rejoué par la chaîne
	chainRepeat int    // répétitions du même verbe (garde anti-boucle)
	chainCode   *int   // code de l'étape terminée, consommé par la re-analyse
}

var theApp *app
var wndProcPtr = windows.NewCallback(wndProc)

func main() {
	if !ensureElevated() {
		return // l'enfant élevé prend le relais (un seul prompt UAC)
	}
	exe, err := os.Executable()
	if err != nil {
		fatalBox("introuvable : " + err.Error())
		return
	}
	dir := filepath.Dir(exe)
	cgo := filepath.Join(dir, "cgo.exe")
	if _, err := os.Stat(cgo); err != nil {
		fatalBox("cgo.exe introuvable à côté de cgo-gui.exe.\nDézippez l'archive complète au même endroit.")
		return
	}
	theApp = &app{
		cgoExe:  cgo,
		cfgPath: filepath.Join(dir, "kit", "cgo-vm.yaml"),
	}
	theApp.appendLog("Meteolink Kit " + kit.KitVersion + " — journal de session")
	if isElevated() {
		theApp.appendLog("élevé : oui — VBox/VMware, réseau hôte et tunnel sans re-prompt")
	} else {
		theApp.appendLog("élevé : non (UAC refusé ou CGO_GUI_NO_ELEVATE) — certaines actions re-demanderont")
	}
	if err := theApp.run(); err != nil {
		fatalBox(err.Error())
	}
}

func fatalBox(msg string) {
	t, _ := windows.UTF16PtrFromString(msg)
	c, _ := windows.UTF16PtrFromString("Meteolink Kit")
	pMsgBox.Call(uintptr(0), uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), uintptr(0x10))
}

// askYes — confirmation Oui/Non, pressions manuelles seules (jamais dans la
// chaîne Suite : un modal sans opérateur devant = deadlock). True = Oui.
func askYes(title, msg string) bool {
	t, _ := windows.UTF16PtrFromString(msg)
	c, _ := windows.UTF16PtrFromString(title)
	var owner uintptr
	if theApp != nil {
		owner = uintptr(theApp.hwnd)
	}
	ret, _, _ := pMsgBox.Call(owner, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), uintptr(0x04|0x20))
	return int(ret) == 6 // IDYES
}
