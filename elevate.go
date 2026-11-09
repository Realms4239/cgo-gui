//go:build windows

package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isElevated — vrai si le processus porte déjà un token administrateur.
func isElevated() bool {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok); err != nil {
		return false
	}
	defer tok.Close()
	var elev uint32
	var outLen uint32
	sz := uint32(4)
	if err := windows.GetTokenInformation(tok, windows.TokenElevation, (*byte)(unsafe.Pointer(&elev)), sz, &outLen); err != nil {
		return false
	}
	return elev != 0
}

// ensureElevated — UN seul prompt UAC au lancement : si le processus n'est
// pas élevé, il se relance lui-même via "runas" puis le parent s'efface.
// Tout l'arbre (cgo.exe, ssh, VBoxManage, powershell) hérite du token —
// plus aucun terminal d'élévation per-action. Retourne false quand le
// processus courant doit s'effacer (l'enfant prend le relais).
// CGO_GUI_NO_ELEVATE=1 : échappatoire (dev/tests, jamais en prod).
func ensureElevated() bool {
	if isElevated() || os.Getenv("CGO_GUI_NO_ELEVATE") == "1" {
		return true
	}
	exe, err := os.Executable()
	if err != nil {
		return true // doute → continue sans élévation, le journal le dira
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return true // refus UAC → continue dégradé, statut l'affiche
	}
	return false
}
