//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Realms4239/cgo-gui/kit"
)

// ---- journal thread-safe (rendu Win32 ; le miroir fichier vit dans core) ----

func (a *app) appendLog(line string) {
	line = time.Now().Format("15:04:05") + " " + line
	a.mu.Lock()
	a.logText = append(a.logText, strings.ReplaceAll(line, "\x00", ""))
	if len(a.logText) > 300 {
		a.logText = a.logText[len(a.logText)-300:]
	}
	a.logDirty = true
	a.mu.Unlock()
	appendGUIFile(line)
}

// flushLog — rendu effectif (5 images/s max), APPEND-ONLY : seul le
// delta depuis le dernier flush est injecté (EM_REPLACESEL en fin),
// O(delta) au lieu de réécrire 120 lignes + reflow complet à chaque tick.
// Reset complet (rare) si la mémoire a tronqué (300 lignes max).
func (a *app) flushLog() {
	a.mu.Lock()
	if !a.logDirty {
		a.mu.Unlock()
		return
	}
	a.logDirty = false
	n := len(a.logText)
	if a.shown > n {
		a.shown = 0 // tronqué entre-temps → reset complet
	}
	var delta string
	full := false
	if a.shown == 0 && n > 0 {
		start := 0
		if n > 120 {
			start = n - 120
		}
		delta = strings.Join(a.logText[start:], "\r\n")
		a.shown = n
		full = true
	} else if a.shown < n {
		end := n
		if end-a.shown > 80 {
			end = a.shown + 80 // borne un tick (le reste au suivant)
		}
		delta = "\r\n" + strings.Join(a.logText[a.shown:end], "\r\n")
		a.shown = end
		if end < n {
			a.logDirty = true
		}
	}
	a.mu.Unlock()
	if delta == "" {
		return
	}
	if full {
		setText(a.logEdit, delta)
	} else {
		sendMsg(a.logEdit, emSetsel, ^uintptr(0)>>1, ^uintptr(0)>>1)
		p := utf16(delta)
		sendMsg(a.logEdit, emReplacesel, 0, uintptr(unsafe.Pointer(p)))
	}
	sendMsg(a.logEdit, emScrollcaret, 0, 0)
}

func (a *app) drainPending() {
	a.mu.Lock()
	q := a.pending
	a.pending = nil
	a.mu.Unlock()
	for _, ln := range q {
		a.appendLog(ln)
	}
}

func (a *app) queueLog(line string) {
	a.mu.Lock()
	a.pending = append(a.pending, line)
	a.mu.Unlock()
	postMsg(a.hwnd, wmAppLog)
}

func (a *app) setStatus(s string) {
	setText(a.status, s)
}

type doneMsg struct {
	label string
	code  int
}

var (
	doneQueue []doneMsg
	doneMu    sync.Mutex
)

func (a *app) onDone() {
	doneMu.Lock()
	var d *doneMsg
	if len(doneQueue) > 0 {
		d = &doneQueue[0]
		doneQueue = doneQueue[1:]
	}
	doneMu.Unlock()
	if d == nil {
		return
	}
	a.setBusy("")
	if d.code == 0 {
		a.appendLog("✓ " + d.label + " terminé")
	} else {
		a.appendLog("✗ " + d.label + " → code " + itoa(d.code) + " (voir ci-dessus)")
	}
	// La chaîne Suite consomme ce code dès la re-analyse terminée.
	a.mu.Lock()
	if a.chain {
		c := d.code
		a.chainCode = &c
	}
	a.mu.Unlock()
	a.setStatus("Prêt.")
	go a.refreshStatus()
	if d.label == "deploy" && d.code == 0 {
		a.appendLog("dashboard : " + a.dashURL())
	}
}

// ---- moteur d'actions ----

type ktWriter struct {
	a *app
}

func (w *ktWriter) Write(p []byte) (int, error) {
	for _, ln := range strings.Split(string(p), "\n") {
		if t := strings.TrimRight(ln, "\r"); strings.TrimSpace(t) != "" {
			w.a.queueLog(t)
		}
	}
	return len(p), nil
}

func (a *app) kitRunner() *kit.Runner {
	r := kit.NewRunner()
	w := &ktWriter{a: a}
	r.Stdout, r.Stderr = w, w
	return r
}

func (a *app) loadCfg() *kit.Config { return a.cfg.load(a.cfgPath) }

func (a *app) noteHost(ip string) { a.cfg.note(ip) }

// setBusy — état occupant + grise/réactive les boutons d'action.
// Appelé depuis le thread UI (handlers) et depuis la goroutine de fin
// de scan — EnableWindow vers nos propres fenêtres est servi par la
// pompe (invariante LockOSThread), donc sans interblocage.
func (a *app) setBusy(label string) {
	a.mu.Lock()
	a.busy = label
	btns := a.btns
	a.mu.Unlock()
	var en uintptr
	if label == "" {
		en = 1
	}
	for _, b := range btns {
		pEnableWindow.Call(uintptr(b), en)
	}
}

// runKit — action kit NON interactive en fond (sortie streamée).
func (a *app) runKit(label string, fn func(r *kit.Runner) int) {
	a.mu.Lock()
	if a.busy != "" {
		busy := a.busy
		a.mu.Unlock()
		a.appendLog("patience — « " + busy + " » tourne déjà")
		return
	}
	a.mu.Unlock()
	a.setBusy(label)
	a.setStatus("◌ " + label + " …")
	a.appendLog("▸ " + label + " …")
	// Bandeau marqué PÉRIMÉ dès le départ : l'action mute le monde (forward
	// posé, VM bootée) et la « prochaine étape » affichée date d'avant
	// (rapport 1.3.4 : bandeau rassis après ensure). Recalculé à la fin.
	a.mu.Lock()
	a.stNext, a.stNextDone = "recalcul après « "+label+" » …", false
	a.mu.Unlock()
	postMsg(a.hwnd, wmAppStatus)
	go func() {
		code := fn(a.kitRunner())
		doneMu.Lock()
		doneQueue = append(doneQueue, doneMsg{label: label, code: code})
		doneMu.Unlock()
		postMsg(a.hwnd, wmAppDone)
	}()
}

// runKitConsole — action INTERACTIVE (mot de passe) : vraie console avec
// CLAVIER, pas un flash. Un exe windowsgui n'a pas de console : spawné en
// CREATE_NEW_CONSOLE direct, l'enfant hérite un stdin invalide/NUL —
// isTerminal() échoue (ou EOF immédiat) et le processus meurt en <1 s sans
// interaction (flash rapporté). `cmd /c start "titre" /wait` donne à
// l'enfant une vraie console conhost (CONIN$ clavier) ; /wait bloque
// jusqu'à fermeture et propage le code de sortie.
func (a *app) runKitConsole(label string, args ...string) {
	a.mu.Lock()
	if a.busy != "" {
		busy := a.busy
		a.mu.Unlock()
		a.appendLog("patience — « " + busy + " » tourne déjà")
		return
	}
	a.mu.Unlock()
	a.setBusy(label)
	a.setStatus("◌ " + label + " (console) …")
	a.appendLog("▸ " + label + " — tapez dans la console noire, fermez-la au retour …")
	// Même marquage périmé que runKit (bandeau d'avant-console rassis).
	a.mu.Lock()
	a.stNext, a.stNextDone = "recalcul après « "+label+" » …", false
	a.mu.Unlock()
	postMsg(a.hwnd, wmAppStatus)
	go func() {
		full := append([]string{"/c", "start", "Meteolink Kit — " + label, "/wait", a.cgoExe, "kit", "--config", a.cfgPath}, args...)
		cmd := exec.Command("cmd.exe", full...)
		cmd.Dir = filepath.Dir(a.cgoExe)
		cmd.Env = append(os.Environ(), "CGO_GUI_CONSOLE=1")
		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = 1
			}
		}
		// Vérifie, ne crois pas (miroir Fyne) : si la console est morte
		// avant la fin, « ✓ terminé » serait un mensonge.
		if code == 0 && label == "poser-clé" {
			if _, err := a.loadCfg().SSH("true"); err != nil {
				a.appendLog("console fermée avant la fin ? clé non vérifiée — relancez « Poser la clé »")
				code = 9
			} else {
				a.appendLog("clé vérifiée : acceptée par l'invité")
			}
		}
		doneMu.Lock()
		doneQueue = append(doneQueue, doneMsg{label: label, code: code})
		doneMu.Unlock()
		postMsg(a.hwnd, wmAppDone)
	}()
}

// ---- rafraîchissements (directs, pas de parsing de sortie) ----

// refreshVMs — corps SYNCHRONE (les appelants préfixent déjà `go`) :
// l'ancienne double-goroutine effaçait `busy` AVANT la fin du scan.
func (a *app) refreshVMs() {
	rows := scanVMRows()
	a.mu.Lock()
	a.vmRows = rows
	a.mu.Unlock()
	postMsg(a.hwnd, wmAppVMs)
	postMsg(a.hwnd, wmAppDone2)
}

// refreshStatus — UNE sonde à la fois (single-flight) : sans garde, un
// clic + le ticker 30 s lançaient DEUX ssh.exe concurrents et le clic
// paraissait « freeze » le temps des handshakes sériels.
func (a *app) refreshStatus() {
	if !a.statFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer a.statFlight.Store(false)
		c := a.loadCfg()
		if c.SSHHost != "" && c.SSHHost != "auto" {
			a.noteHost(c.SSHHost)
		}
		ssh, dash, locked := probeStatus(c)
		a.mu.Lock()
		a.stSSH, a.stDash, a.stLocked = ssh, dash, locked
		a.mu.Unlock()
		postMsg(a.hwnd, wmAppStatus)
		// Prochaine étape : coûteux (~10-25 s), calculé APRÈS l'affichage
		// du statut pour ne pas le retarder, même single-flight.
		// kit.KitVersion : un dashboard sain mais périmé ne passe pas les
		// paliers neufs en silence (remède : deploy).
		nx := kit.FirstOpen(kit.GatherNextVer(c, kit.KitVersion))
		a.mu.Lock()
		if nx == nil {
			a.stNext, a.stNextKind, a.stNextArgs = "tout est vert — dashboard prêt", "", nil
		} else if nx.Remedy != "" {
			// Verbe vide (pilote manquant, update locale…) = "none" :
			// nk=="" reste RÉSERVÉ au vrai tout-vert, sinon Suite ment.
			verb := nx.Verb
			if verb == "" {
				verb = "none"
			}
			a.stNext, a.stNextKind, a.stNextArgs = nx.Label+" — "+nx.Remedy, verb, nx.Args
		} else {
			verb := nx.Verb
			if verb == "" {
				verb = "none"
			}
			a.stNext, a.stNextKind, a.stNextArgs = nx.Label+" ("+nx.Detail+")", verb, nx.Args
		}
		a.stNextDone = true
		var cc *int
		if a.chain && a.chainCode != nil {
			cc = a.chainCode
			a.chainCode = nil
		}
		a.mu.Unlock()
		postMsg(a.hwnd, wmAppStatus)
		if cc != nil {
			a.suiteContinue(*cc)
		}
	}()
}

// portOpenGUI — dial TCP court, miroir kit.portOpen (non exporté là-bas).
// ---- dispatch boutons ----

// buttonAction — mapping pur bouton → action (testé sans fenêtre) :
// retourne le verbe et ses args CLI. "console:" = console visible,
// "bg:" = fond streamé, "direct:" = appel immédiat (verrouillage).
func (a *app) onButton(id int) {
	kind, args := buttonAction(id)
	if kind == "" {
		// JAMAIS muet : un id sans verbe (désync layout/map, clic fantôme)
		// ressemblait à un bouton mort sans aucune trace (rapport 1.3.1).
		a.appendLog("clic sans action (bouton " + itoa(id) + ") — signalez-le avec ce journal")
		return
	}
	// Main humaine directe (pas Suite) : la chaîne s'efface, pas de reprise
	// surprise après l'action manuelle.
	if kind != "direct:suite" {
		a.mu.Lock()
		a.chain = false
		a.mu.Unlock()
		// Destructrices : confirmation MAIN HUMAINE (jamais dans la chaîne —
		// un modal sans opérateur devant = deadlock).
		switch kind {
		case "bg:nic-toggle":
			if !askYes("Carte réseau", "Basculer NAT ↔ pont sur une VM allumée peut l'éteindre. Continuer ?") {
				a.appendLog("carte réseau : annulé — rien touché")
				return
			}
		case "bg:vmoff":
			if !askYes("Arrêter la VM", "Arrêter la VM ? (campagnes et dashboard en cours seront coupés)") {
				a.appendLog("arrêt VM : annulé — rien touché")
				return
			}
		}
	}
	a.runKind(kind, args)
}

// runKind — exécute un verbe (bouton ou « Suite » qui rejoue le verbe de
// la prochaine étape calculée). Factorisé de onButton pour que Suite
// emprunte exactement le même chemin que le bouton d'origine.
func (a *app) runKind(kind string, args []string) {
	switch {
	case kind == "direct:lock":
		a.lockSelected()
	case kind == "direct:saveuser":
		a.saveUser()
	case kind == "direct:open":
		openBrowser(a.dashURL())
		a.appendLog("navigateur → " + a.dashURL())
	case kind == "bg:scan":
		a.appendLog("▸ scan des VMs …")
		a.setBusy("scan")
		a.setStatus("◌ scan …")
		go func() {
			a.refreshVMs()
			// Verrouillage auto si non ambigu : Suite avance seul depuis
			// une config vierge (sinon re-scan en boucle sans progresser).
			if msg, ok := kit.AutoLockSingle(a.loadCfg(), a.cfgPath); ok {
				a.appendLog(msg)
			} else if msg != "" {
				a.appendLog(msg)
			}
			a.setBusy("")
			go a.refreshStatus()
			// PAS de wmAppDone2 ici : refreshVMs le poste déjà
			// (wmAppVMs + wmAppDone2) — doublon = « ✓ scan terminé » × 2.
		}()
	case kind == "bg:diag":
		a.appendLog("diagnostic : clé, port, auth, IP — voir journal")
		a.runKit("diagnostic", func(r *kit.Runner) int { return diagGUI(a.loadCfg(), r) })
	case kind == "bg:dns":
		a.runKit("dns", func(r *kit.Runner) int { return r.DNS(a.loadCfg()) })
	case kind == "bg:tls":
		a.runKit("tls", func(r *kit.Runner) int { return r.TLS(a.loadCfg()) })
	case kind == "bg:mkkey":
		a.runKit("créer-clé", func(r *kit.Runner) int { return mkKeyGUI(a.loadCfg(), r) })
	case kind == "bg:console":
		a.runKit("console", func(r *kit.Runner) int { return openConsoleGUI(a.loadCfg(), r) })
	case kind == "bg:ensure":
		a.runKit("ensure", func(r *kit.Runner) int { return r.Ensure(a.loadCfg(), a.cfgPath, true) })
	case kind == "bg:deploy":
		a.runKit("deploy", func(r *kit.Runner) int { return r.Deploy(a.loadCfg(), a.cfgPath, true) })
	case kind == "bg:svc":
		sub := "status"
		if len(args) > 0 {
			sub = args[0]
		}
		a.runKit("svc "+sub, func(r *kit.Runner) int { return r.Svc(a.loadCfg(), sub) })
	case kind == "bg:testbed":
		a.runKit("testbed", func(r *kit.Runner) int { return r.Testbed(a.loadCfg(), args) })
	case kind == "bg:logs":
		a.runKit("logs", func(r *kit.Runner) int {
			c := a.loadCfg()
			if c.VMPath() == "" && c.SSHHost == "" {
				a.appendLog("rien à lire : verrouillez d'abord une VM (liste ci-dessus)")
				return 3
			}
			return r.Logs(c, 40)
		})
	case kind == "bg:verify":
		a.runKit("vérifier", func(r *kit.Runner) int {
			if code := r.Verify(a.loadCfg()); code != 0 {
				return code
			}
			// Porte d'embarquement dans la foulée (DNS→TLS→health) : le
			// manuel l'exige après chaque deploy, un seul bouton suffit.
			return r.ShipCheck(a.loadCfg())
		})
	case kind == "bg:backup":
		a.runKit("backup", func(r *kit.Runner) int { return r.Backup(a.loadCfg(), "backup") })
	case kind == "bg:snapshot":
		a.runKit("snapshot", func(r *kit.Runner) int { return r.Snapshot(a.loadCfg(), "cgo-gui", true) })
	case kind == "bg:netinfo":
		a.runKit("netinfo", func(r *kit.Runner) int { return r.Netinfo(a.loadCfg()) })
	case kind == "bg:vmon":
		a.runKit("vm-start", func(r *kit.Runner) int { return vmPowerGUI(a.loadCfg(), r, true) })
	case kind == "bg:vmoff":
		a.runKit("vm-stop", func(r *kit.Runner) int { return vmPowerGUI(a.loadCfg(), r, false) })
	case kind == "bg:nic-toggle":
		a.runKit("nic-toggle", func(r *kit.Runner) int { return nicToggleGUI(a.loadCfg(), r, a.cfgPath) })
	case kind == "bg:nic":
		a.runKit("nic-nat", func(r *kit.Runner) int { return r.Nic(a.loadCfg(), a.cfgPath, args, true) })
	case kind == "bg:hosttun":
		a.runKit("tunnel-hôte", func(r *kit.Runner) int {
			c := a.loadCfg()
			_, argv, blocked := hostTunCmd(c, filepath.Dir(a.cgoExe), r.Root)
			if blocked != "" {
				// journal, JAMAIS stdout : sous windowsgui, fmt.Println part
				// dans le vide et le bouton passe pour mort (vu en prod).
				a.appendLog(blocked)
				return 3
			}
			cmd := kit.BgCmd("powershell", argv...)
			cmd.Stdout, cmd.Stderr = r.Stdout, r.Stderr
			if err := cmd.Run(); err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					return ee.ExitCode()
				}
				return 1
			}
			return 0
		})
	case kind == "bg:guest":
		a.runKit("invité", func(r *kit.Runner) int { return r.Guest(a.loadCfg(), a.cfgPath, false) })
	case kind == "bg:vnet":
		a.runKit("réseau-hôte", func(r *kit.Runner) int { return r.Vnet(a.loadCfg()) })
	case kind == "console:keysetup":
		a.runKitConsole("poser-clé", "keysetup")
	case kind == "console:dns":
		a.runKitConsole("dns", "dns")
	case kind == "console:tls":
		a.runKitConsole("tls", "tls")
	case kind == "bg:guide":
		a.runKit("guide", func(r *kit.Runner) int {
			for _, ln := range strings.Split(kit.Readme(), "\n") {
				fmt.Fprintln(r.Stdout, ln)
			}
			return 0
		})
	case kind == "direct:suite":
		a.mu.Lock()
		done, nk, na := a.stNextDone, a.stNextKind, a.stNextArgs
		next := a.stNext
		a.mu.Unlock()
		if !done {
			a.appendLog("analyse en cours — patientez 10 s puis Suite")
			return
		}
		// Chaîne linéaire : UNE pression avance seule jusqu'au premier point
		// qui exige une main humaine (console, choix, install) ou échoue.
		a.mu.Lock()
		a.chain = true
		a.chainVerb = ""
		a.chainRepeat = 0
		a.mu.Unlock()
		a.suiteStep(nk, na, next)
	}
}

// suiteStep — UN maillon de la chaîne (Suite initial ou continuation auto
// après chaque étape réussie). Ne boucle jamais : arrêt sur console (main
// humaine requise), verbe inconnu, échec, vert, ou même verbe 2× de suite.
func (a *app) suiteStep(nk string, na []string, next string) {
	if nk == "" {
		a.mu.Lock()
		a.chain = false
		a.mu.Unlock()
		a.appendLog("tout est vert — rien à faire (Rescanner pour revérifier)")
		return
	}
	if !isKnownSuiteVerb(nk) {
		a.mu.Lock()
		a.chain = false
		a.mu.Unlock()
		// Palier sans verbe auto (pilote manquant, update locale…) :
		// JAMAIS muet, JAMAIS « tout est vert » — le remède est dans
		// le bandeau, on le répète ici.
		a.appendLog("suite : pas d'action automatique — " + next)
		return
	}
	if strings.HasPrefix(nk, "console:") {
		a.mu.Lock()
		a.chain = false
		a.mu.Unlock()
		a.appendLog("suite : à vous — tapez dans la console noire, fermez-la, puis Suite pour continuer")
		a.runKind(nk, na)
		return
	}
	a.mu.Lock()
	if nk == a.chainVerb {
		a.chainRepeat++
	} else {
		a.chainVerb = nk
		a.chainRepeat = 0
	}
	repeat := a.chainRepeat
	a.mu.Unlock()
	if repeat >= 1 {
		a.mu.Lock()
		a.chain = false
		a.mu.Unlock()
		a.appendLog("suite : « " + nk + " » n'a pas fait avancer le bandeau — j'arrête là, lisez le journal ci-dessus")
		return
	}
	a.appendLog("▶ suite : " + nk)
	a.runKind(nk, na)
}

// suiteContinue — appelée en fin de re-analyse (refreshStatus) quand une
// chaîne est en cours : l'étape vient de se terminer (code en journal via
// onDone), on avance au maillon suivant si elle a réussi.
func (a *app) suiteContinue(code int) {
	a.mu.Lock()
	chain := a.chain
	done, nk, na := a.stNextDone, a.stNextKind, a.stNextArgs
	next := a.stNext
	a.mu.Unlock()
	if !chain || !done {
		return
	}
	if code != 0 {
		a.mu.Lock()
		a.chain = false
		a.mu.Unlock()
		a.appendLog("suite : pause — l'étape a échoué (remède dans le journal ci-dessus), corrigez puis Suite")
		return
	}
	a.suiteStep(nk, na, next)
}

func (a *app) dashURL() string {
	host, port := "meteolink.dev", "9090"
	// loadFast : JAMAIS de GuestIP ici — dashURL tourne sur le thread UI
	// (clic « Ouvrir le dashboard »), LoadConfig résoudrait via vmrun.
	if c := a.cfg.loadFast(a.cfgPath); c != nil {
		if c.DashHost != "" {
			host = c.DashHost
		}
		if c.DashPort != "" {
			port = c.DashPort
		}
	}
	return "https://" + host + ":" + port
}
