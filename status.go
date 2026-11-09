//go:build windows

package main

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/Realms4239/cgo-gui/kit"
	"github.com/Realms4239/cgo-gui/vm"
)

// ---- VM list ----

func (a *app) onVMs() {
	a.mu.Lock()
	rows := a.vmRows
	a.mu.Unlock()
	sendMsg(a.vmList, lbResetcontent, 0, 0)
	for _, r := range rows {
		dot := "○"
		if r.live {
			dot = "●"
		}
		mode := r.mode
		if mode == "" || mode == "inconnu" {
			mode = "?"
		}
		lbl := fmt.Sprintf("%s %s [%s · %s]", dot, r.name, r.hyp, mode)
		sendMsg(a.vmList, lbAddstring, 0, ptr(lbl))
	}
	a.appendLog(fmt.Sprintf("%d VM(s) — double-clic = verrouiller", len(rows)))
}

func ptr(s string) uintptr {
	return uintptr(unsafe.Pointer(utf16(s)))
}

func (a *app) lockSelected() {
	idx := int(sendMsg(a.vmList, lbGetcursel, 0, 0))
	a.mu.Lock()
	rows := a.vmRows
	a.mu.Unlock()
	if len(rows) == 0 {
		a.appendLog("liste vide — « Lister » d'abord, puis double-clic sur la VM")
		return
	}
	if idx < 0 || idx >= len(rows) {
		// LB_ERR (aucun curseur, ex. double-clic rapide) : à VM unique,
		// pas d'ambiguïté — la prendre plutôt que passer pour morte
		// (rapport 1.3.4 : double-clic-pour-verrouiller peu fiable).
		if len(rows) == 1 {
			idx = 0
		} else {
			a.appendLog("cliquez d'abord une VM dans la liste, puis Verrouiller")
			return
		}
	}
	v := rows[idx]
	// Pilote par l'extension (source unique : vm.HypForPath — jamais la
	// devinette locale qui a déjà envoyé des .vbox chez vmrun).
	hyp := vm.HypForPath(v.path)
	if hyp == "" {
		hyp = v.hyp
	}
	if err := kit.SaveVMX(a.cfgPath, v.path, hyp); err != nil {
		a.appendLog("verrouillage : " + err.Error())
		return
	}
	setText(a.locked, "Verrouillée : "+v.name+" ("+hyp+")")
	a.appendLog("verrouillée : " + v.name)
	go a.refreshStatus()
}

// ---- statuts ----

func (a *app) onStatus() {
	a.mu.Lock()
	ssh, dash, locked := a.stSSH, a.stDash, a.stLocked
	next := a.stNext
	done := a.stNextDone
	prevSSH, prevDash := a.lastSSH, a.lastDash
	if ssh != prevSSH {
		a.lastSSH = ssh
	}
	if dash != prevDash {
		a.lastDash = dash
	}
	a.mu.Unlock()
	setText(a.sshLbl, "SSH : "+orDash(ssh))
	setText(a.dashLbl, "Dashboard : "+orDash(dash))
	// État journalisé sur TRANSITIONS seules (le ticker 30 s ne spamme pas) :
	// un bouton « mort » vs « pas encore applicable » se lit dans le journal.
	if ssh != prevSSH && ssh != "" {
		a.appendLog("état SSH : " + ssh)
	}
	if dash != prevDash && dash != "" {
		a.appendLog("état dashboard : " + dash)
	}
	if locked != "" {
		setText(a.locked, "Verrouillée : "+locked)
	}
	if done && next != "" {
		setText(a.nextLbl, "→ Prochaine : "+next)
		a.mu.Lock()
		changed := a.lastNext != next
		if changed {
			a.lastNext = next
		}
		a.mu.Unlock()
		if changed {
			a.appendLog("→ Prochaine : " + next)
		}
	} else if !done {
		setText(a.nextLbl, "→ Prochaine : calcul en cours…")
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (a *app) onScanDone() {
	a.setBusy("")
	a.setStatus("Prêt.")
	a.appendLog("✓ scan terminé")
}

// saveUser — enregistre l'utilisateur Ubuntu tapé (le chaînon manquant :
// sans ssh_user, diagnostic + invité + deploy avortent avec des messages
// qui n'expliquent pas où le renseigner — ICI). Commit d'abord : le focus
// quitte le champ AVANT lecture, sinon le premier clic Sauver lisait un
// champ pas encore validé et passait pour avalé.
func (a *app) saveUser() {
	pSetFocus.Call(uintptr(a.hwnd))
	u := strings.TrimSpace(getText(a.userEdit))
	if u == "" {
		a.appendLog("utilisateur vide — tapez le nom Ubuntu (ex. fanasina) puis Sauver")
		return
	}
	if err := kit.SaveSSHTarget(a.cfgPath, u, "", "", ""); err != nil {
		a.appendLog("sauvegarde : " + err.Error())
		return
	}
	a.appendLog("utilisateur SSH : " + u + " — Diagnostiquer pour vérifier")
	go a.refreshStatus()
}
