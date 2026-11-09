//go:build linux && cgo

// Meteolink Kit — vue Linux (Fyne) du même coeur que la vue Win32.
// Même boutons, même journal, même garde anti-double-clic : seul le
// rendu change. Sans écran (DISPLAY/WAYLAND_DISPLAY vides : SSH, serveur)
// l'app refuse poliment et renvoie vers `cgo kit tui`.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Realms4239/cgo-gui/kit"
)

type fyApp struct {
	app    fyne.App
	win    fyne.Window
	cfg    cfgCache
	cfgPth string
	cgoExe string

	mu       sync.Mutex
	busy     string
	rows     []vmRow
	sel      int
	logLines []string

	logCh       chan string
	journal     *widget.Entry
	jScroll     *container.Scroll
	status      *widget.Label
	nextLbl     *widget.Label
	nextKind    string
	nextArgs    []string
	nextDone    bool
	lastNext    string
	chain       bool
	chainVerb   string
	chainRepeat int
	chainCode   *int
	sshLbl      *widget.Label
	userEdit    *widget.Entry
	dashLbl     *widget.Label
	lockLbl     *widget.Label
	vmList      *widget.List

	statFlight atomic.Bool
	tick       uint64
}

func (f *fyApp) loadCfg() *kit.Config { return f.cfg.load(f.cfgPth) }
func (f *fyApp) noteHost(ip string)   { f.cfg.note(ip) }

func (f *fyApp) log(line string) {
	select {
	case f.logCh <- line:
	default:
	}
	appendGUIFile(line)
}

func (f *fyApp) setStatus(s string) { f.status.SetText(s) }

// pump — 5 images/s : vide le canal vers le journal (150 dernières
// lignes), balaye SSH/dashboard toutes les ~30 s. Même rythme que Win32.
func (f *fyApp) pump() {
	tk := time.NewTicker(200 * time.Millisecond)
	defer tk.Stop()
	for range tk.C {
		f.tick++
		drained := false
		for {
			select {
			case ln := <-f.logCh:
				f.mu.Lock()
				f.logLines = append(f.logLines, ln)
				if len(f.logLines) > 300 {
					f.logLines = f.logLines[len(f.logLines)-300:]
				}
				f.mu.Unlock()
				drained = true
			default:
				goto flushed
			}
		}
	flushed:
		if drained {
			f.mu.Lock()
			start := 0
			if len(f.logLines) > 150 {
				start = len(f.logLines) - 150
			}
			text := strings.Join(f.logLines[start:], "\n")
			n := len(f.logLines)
			f.mu.Unlock()
			f.journal.SetText(text)
			f.journal.CursorRow = n
			f.journal.Refresh()
			f.jScroll.ScrollToBottom()
		}
		if f.tick%150 == 0 {
			f.refreshStatus()
		}
	}
}

func (f *fyApp) refreshStatus() {
	if !f.statFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer f.statFlight.Store(false)
		c := f.loadCfg()
		if c.SSHHost != "" && c.SSHHost != "auto" {
			f.noteHost(c.SSHHost)
		}
		ssh, dash, locked := probeStatus(c)
		f.sshLbl.SetText("SSH : " + orDashF(ssh))
		f.dashLbl.SetText("Dashboard : " + orDashF(dash))
		if locked != "" {
			f.lockLbl.SetText("Verrouillée : " + locked)
		}
		nx := kit.FirstOpen(kit.GatherNextVer(c, kit.KitVersion))
		f.mu.Lock()
		var banner, nk string
		var na []string
		if nx == nil {
			banner = "tout est vert — dashboard prêt"
		} else if nx.Remedy != "" {
			banner = nx.Label + " — " + nx.Remedy
			nk, na = nx.Verb, nx.Args
		} else {
			banner = nx.Label + " (" + nx.Detail + ")"
			nk, na = nx.Verb, nx.Args
		}
		// nk=="" réservé au vrai tout-vert (miroir Win32) : les paliers
		// sans verbe auto deviennent "none" → message honnête, jamais vert.
		if nx != nil && nk == "" {
			nk = "none"
		}
		changed := f.lastNext != banner
		if changed {
			f.lastNext = banner
		}
		f.nextKind, f.nextArgs = nk, na
		f.nextDone = true
		var cc *int
		if f.chain && f.chainCode != nil {
			cc = f.chainCode
			f.chainCode = nil
		}
		f.mu.Unlock()
		f.nextLbl.SetText("→ Prochaine : " + banner)
		if changed {
			f.log("→ Prochaine : " + banner)
		}
		if cc != nil {
			f.suiteContinue(*cc)
		}
	}()
}

func orDashF(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

type fyWriter struct{ f *fyApp }

func (w *fyWriter) Write(p []byte) (int, error) {
	for _, ln := range strings.Split(string(p), "\n") {
		if t := strings.TrimRight(ln, "\r"); strings.TrimSpace(t) != "" {
			w.f.log(t)
		}
	}
	return len(p), nil
}

func (f *fyApp) kitRunner() *kit.Runner {
	r := kit.NewRunner()
	w := &fyWriter{f: f}
	r.Stdout, r.Stderr = w, w
	return r
}

// runBg — action kit en fond, UNE à la fois (même garde que Win32).
func (f *fyApp) runBg(label string, fn func(r *kit.Runner) int) {
	f.mu.Lock()
	if f.busy != "" {
		busy := f.busy
		f.mu.Unlock()
		f.log("patience — « " + busy + " » tourne déjà")
		return
	}
	f.busy = label
	f.mu.Unlock()
	f.setStatus("◌ " + label + " …")
	f.log("▸ " + label + " …")
	// Bandeau périmé dès le départ (miroir Win32) : recalculé à la fin.
	f.mu.Lock()
	f.lastNext = "recalcul après « " + label + " » …"
	f.nextDone = false
	f.mu.Unlock()
	f.nextLbl.SetText("→ Prochaine : recalcul après « " + label + " » …")
	go func() {
		code := fn(f.kitRunner())
		f.mu.Lock()
		f.busy = ""
		f.mu.Unlock()
		if code == 0 {
			f.log("✓ " + label + " terminé")
		} else {
			f.log(fmt.Sprintf("✗ %s → code %d (voir ci-dessus)", label, code))
		}
		f.setStatus("Prêt.")
		if label == "deploy" && code == 0 {
			f.log("dashboard : https://meteolink.dev:9090")
		}
		f.mu.Lock()
		if f.chain {
			c := code
			f.chainCode = &c
		}
		f.mu.Unlock()
		f.refreshStatus()
	}()
}

// runTerm — action INTERACTIVE (mot de passe) : émulateur de terminal
// visible, repli = la commande exacte à taper à la main.
func (f *fyApp) runTerm(label string, args ...string) {
	f.mu.Lock()
	if f.busy != "" {
		busy := f.busy
		f.mu.Unlock()
		f.log("patience — « " + busy + " » tourne déjà")
		return
	}
	f.busy = label
	f.mu.Unlock()
	f.setStatus("◌ " + label + " (terminal) …")
	f.log("▸ " + label + " — tapez dans le terminal, fermez-le au retour …")
	// Même marquage périmé que runBg.
	f.mu.Lock()
	f.lastNext = "recalcul après « " + label + " » …"
	f.nextDone = false
	f.mu.Unlock()
	f.nextLbl.SetText("→ Prochaine : recalcul après « " + label + " » …")
	full := append([]string{"kit", "--config", f.cfgPth}, args...)
	go func() {
		code := termRun(f.cgoExe, full)
		// Vérifie, ne crois pas : certains émulateurs (gnome-terminal)
		// rendent la main dès la délégation — « ✓ terminé » serait un
		// mensonge si la clé n'est pas posée.
		if code == 0 && label == "poser-clé" {
			if out, err := f.loadCfg().SSH("true"); err != nil {
				f.log("terminal fermé avant la fin ? clé non vérifiée — relancez « Poser la clé »")
				_ = out
				code = 9
			} else {
				f.log("clé vérifiée : acceptée par l'invité")
			}
		}
		f.mu.Lock()
		f.busy = ""
		f.mu.Unlock()
		if code == 0 {
			f.log("✓ " + label + " terminé")
		} else {
			f.log(fmt.Sprintf("✗ %s → code %d", label, code))
		}
		f.setStatus("Prêt.")
		f.refreshStatus()
	}()
}

// termRun — premier émulateur trouvé, sinon la commande à taper.
func termRun(cgo string, args []string) int {
	type term struct {
		bin  string
		flag string
	}
	for _, t := range []term{
		{"x-terminal-emulator", "-e"}, {"gnome-terminal", "--"},
		{"konsole", "-e"}, {"xfce4-terminal", "-e"}, {"xterm", "-e"},
	} {
		if _, err := exec.LookPath(t.bin); err != nil {
			continue
		}
		cmd := exec.Command(t.bin, append([]string{t.flag, cgo}, args...)...)
		if err := cmd.Start(); err != nil {
			continue
		}
		if err := cmd.Wait(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			return 1
		}
		return 0
	}
	return 9
}

func (f *fyApp) dashURL() string {
	host, port := "meteolink.dev", "9090"
	if c := f.loadCfg(); c != nil {
		if c.DashHost != "" {
			host = c.DashHost
		}
		if c.DashPort != "" {
			port = c.DashPort
		}
	}
	return "https://" + host + ":" + port
}

func (f *fyApp) lockSelected() {
	f.mu.Lock()
	idx, rows := f.sel, f.rows
	f.mu.Unlock()
	if len(rows) == 0 {
		f.log("liste vide — Rescanner d'abord, puis sélectionnez la VM")
		return
	}
	if idx < 0 || idx >= len(rows) {
		// Miroir Win32 : à VM unique, pas d'ambiguïté — la prendre.
		if len(rows) == 1 {
			idx = 0
		} else {
			f.log("sélectionnez d'abord une VM dans la liste")
			return
		}
	}
	v := rows[idx]
	hyp := v.hyp
	if hyp == "?" {
		if strings.HasSuffix(strings.ToLower(v.path), ".vbox") {
			hyp = "virtualbox"
		} else {
			hyp = "vmware"
		}
	}
	if err := kit.SaveVMX(f.cfgPth, v.path, hyp); err != nil {
		f.log("verrouillage : " + err.Error())
		return
	}
	f.lockLbl.SetText("Verrouillée : " + v.name + " (" + hyp + ")")
	f.log("verrouillée : " + v.name)
	f.refreshStatus()
}

func (f *fyApp) scan() {
	f.mu.Lock()
	if f.busy != "" {
		b := f.busy
		f.mu.Unlock()
		f.log("patience — « " + b + " » tourne déjà")
		return
	}
	f.busy = "scan"
	f.mu.Unlock()
	f.setStatus("◌ scan …")
	f.log("▸ scan des VMs …")
	go func() {
		rows := scanVMRows()
		f.mu.Lock()
		f.rows = rows
		// Sélection réinitialisée : les lignes ont été remplacées — garder
		// l'ancien index verrouillerait une AUTRE VM que l'affichée.
		f.sel = -1
		f.busy = ""
		f.mu.Unlock()
		f.vmList.UnselectAll()
		f.vmList.Refresh()
		f.setStatus("Prêt.")
		f.log(fmt.Sprintf("%d VM(s) — sélectionnez puis Verrouiller", len(rows)))
		// Verrouillage auto si non ambigu (miroir Win32) + re-analyse pour
		// que Suite avance seul depuis une config vierge (le refreshStatus
		// ci-dessous remet à jour le bandeau de verrouillage).
		if msg, ok := kit.AutoLockSingle(f.loadCfg(), f.cfgPth); ok {
			f.log(msg)
		} else if msg != "" {
			f.log(msg)
		}
		go f.refreshStatus()
	}()
}

func (f *fyApp) dispatch(id int) {
	kind, args := buttonAction(id)
	if kind == "" {
		return
	}
	// Main humaine directe (pas Suite) : la chaîne s'efface.
	if kind != "direct:suite" {
		f.mu.Lock()
		f.chain = false
		f.mu.Unlock()
	}
	f.dispatchKind(kind, args)
}

// dispatchKind — même factorisation que Win32 runKind : « Suite » rejoue
// le verbe de l'étape calculée par le même chemin que son bouton.
func (f *fyApp) dispatchKind(kind string, args []string) {
	switch {
	case kind == "direct:lock":
		f.lockSelected()
	case kind == "direct:saveuser":
		u := strings.TrimSpace(f.userEdit.Text)
		if u == "" {
			f.log("utilisateur vide — tapez le nom Ubuntu puis Sauver")
			return
		}
		if err := kit.SaveSSHTarget(f.cfgPth, u, "", "", ""); err != nil {
			f.log("sauvegarde : " + err.Error())
			return
		}
		f.log("utilisateur SSH : " + u + " — Diagnostiquer pour vérifier")
		f.refreshStatus()
	case kind == "direct:open":
		if _, err := exec.LookPath("xdg-open"); err != nil {
			f.log("xdg-open absent — ouvrez à la main : " + f.dashURL())
			return
		}
		_ = exec.Command("xdg-open", f.dashURL()).Start()
		f.log("navigateur → " + f.dashURL())
	case kind == "bg:scan":
		f.scan()
	case kind == "bg:diag":
		f.log("diagnostic : clé, port, auth, IP — voir journal")
		f.runBg("diagnostic", func(r *kit.Runner) int { return diagGUI(f.loadCfg(), r) })
	case kind == "bg:dns":
		f.runBg("dns", func(r *kit.Runner) int { return r.DNS(f.loadCfg()) })
	case kind == "bg:tls":
		f.runBg("tls", func(r *kit.Runner) int { return r.TLS(f.loadCfg()) })
	case kind == "bg:mkkey":
		f.runBg("créer-clé", func(r *kit.Runner) int { return mkKeyGUI(f.loadCfg(), r) })
	case kind == "bg:console":
		f.runBg("console", func(r *kit.Runner) int { return openConsoleGUI(f.loadCfg(), r) })
	case kind == "bg:ensure":
		f.runBg("ensure", func(r *kit.Runner) int { return r.Ensure(f.loadCfg(), f.cfgPth, true) })
	case kind == "bg:deploy":
		f.runBg("deploy", func(r *kit.Runner) int { return r.Deploy(f.loadCfg(), f.cfgPth, true) })
	case kind == "bg:svc":
		sub := "status"
		if len(args) > 0 {
			sub = args[0]
		}
		f.runBg("svc "+sub, func(r *kit.Runner) int { return r.Svc(f.loadCfg(), sub) })
	case kind == "bg:testbed":
		f.runBg("testbed", func(r *kit.Runner) int { return r.Testbed(f.loadCfg(), args) })
	case kind == "bg:logs":
		f.runBg("logs", func(r *kit.Runner) int {
			c := f.loadCfg()
			if c.VMPath() == "" && c.SSHHost == "" {
				fmt.Fprintln(r.Stdout, "rien à lire : verrouillez d'abord une VM (liste ci-dessus)")
				return 3
			}
			return r.Logs(c, 40)
		})
	case kind == "bg:verify":
		f.runBg("verify", func(r *kit.Runner) int {
			if code := r.Verify(f.loadCfg()); code != 0 {
				return code
			}
			return r.ShipCheck(f.loadCfg())
		})
	case kind == "bg:backup":
		f.runBg("backup", func(r *kit.Runner) int { return r.Backup(f.loadCfg(), "backup") })
	case kind == "bg:snapshot":
		f.runBg("snapshot", func(r *kit.Runner) int { return r.Snapshot(f.loadCfg(), "cgo-gui", true) })
	case kind == "bg:netinfo":
		f.runBg("netinfo", func(r *kit.Runner) int { return r.Netinfo(f.loadCfg()) })
	case kind == "bg:vmon":
		f.runBg("vm-start", func(r *kit.Runner) int { return vmPowerGUI(f.loadCfg(), r, true) })
	case kind == "bg:vmoff":
		f.runBg("vm-stop", func(r *kit.Runner) int { return vmPowerGUI(f.loadCfg(), r, false) })
	case kind == "bg:nic-toggle":
		f.runBg("nic-toggle", func(r *kit.Runner) int { return nicToggleGUI(f.loadCfg(), r, f.cfgPth) })
	case kind == "bg:nic":
		f.runBg("nic-nat", func(r *kit.Runner) int { return r.Nic(f.loadCfg(), f.cfgPth, args, true) })
	case kind == "bg:hosttun":
		// Linux : pas de .ps1 — la même chaîne en Go natif (hosts,
		// confiance, santé, navigateur) via les actions kit existantes.
		f.runBg("tunnel-hôte", func(r *kit.Runner) int {
			c := f.loadCfg()
			if code := r.DNS(c); code != 0 {
				return code
			}
			if code := r.TLS(c); code != 0 {
				return code
			}
			if ver, ok := dashHealth(c); ok {
				fmt.Fprintln(r.Stdout, "dashboard sain (version "+ver+")")
				if _, err := exec.LookPath("xdg-open"); err != nil {
					fmt.Fprintln(r.Stdout, "xdg-open absent — ouvrez à la main : "+f.dashURL())
				} else {
					_ = exec.Command("xdg-open", f.dashURL()).Start()
				}
				return 0
			}
			fmt.Fprintln(r.Stdout, "dashboard injoignable — `Démarrer / Réessayer` puis relancez")
			return 5
		})
	case kind == "bg:guest":
		f.runBg("invité", func(r *kit.Runner) int { return r.Guest(f.loadCfg(), f.cfgPth, false) })
	case kind == "bg:vnet":
		f.runBg("réseau-hôte", func(r *kit.Runner) int { return r.Vnet(f.loadCfg()) })
	case kind == "console:keysetup":
		f.runTerm("poser-clé", "keysetup")
	case kind == "console:dns":
		f.runTerm("dns", "dns")
	case kind == "console:tls":
		f.runTerm("tls", "tls")
	case kind == "bg:guide":
		f.runBg("guide", func(r *kit.Runner) int {
			for _, ln := range strings.Split(kit.Readme(), "\n") {
				fmt.Fprintln(r.Stdout, ln)
			}
			return 0
		})
	case kind == "direct:suite":
		f.mu.Lock()
		done, nk, na := f.nextDone, f.nextKind, f.nextArgs
		next := f.lastNext
		f.mu.Unlock()
		if !done {
			f.log("analyse en cours — patientez 10 s puis Suite")
			return
		}
		if nk == "" {
			f.log("tout est vert — rien à faire (Rescanner pour revérifier)")
			return
		}
		if !isKnownSuiteVerb(nk) {
			f.log("suite : pas d'action automatique — " + next)
			return
		}
		f.mu.Lock()
		f.chain = true
		f.chainVerb = ""
		f.chainRepeat = 0
		f.mu.Unlock()
		f.suiteStep(nk, na, next)
	}
}

func (f *fyApp) suiteStep(nk string, na []string, next string) {
	if nk == "" {
		f.mu.Lock()
		f.chain = false
		f.mu.Unlock()
		f.log("tout est vert — rien à faire (Rescanner pour revérifier)")
		return
	}
	if !isKnownSuiteVerb(nk) {
		f.mu.Lock()
		f.chain = false
		f.mu.Unlock()
		f.log("suite : pas d'action automatique — " + next)
		return
	}
	if len(nk) >= 8 && nk[:8] == "console:" {
		f.mu.Lock()
		f.chain = false
		f.mu.Unlock()
		f.log("suite : à vous — tapez dans le terminal, puis Suite pour continuer")
		f.dispatchKind(nk, na)
		return
	}
	f.mu.Lock()
	if nk == f.chainVerb {
		f.chainRepeat++
	} else {
		f.chainVerb = nk
		f.chainRepeat = 0
	}
	repeat := f.chainRepeat
	f.mu.Unlock()
	if repeat >= 1 {
		f.mu.Lock()
		f.chain = false
		f.mu.Unlock()
		f.log("suite : « " + nk + " » n'a pas fait avancer le bandeau — j'arrête là, lisez le journal ci-dessus")
		return
	}
	f.log("▶ suite : " + nk)
	f.dispatchKind(nk, na)
}

func (f *fyApp) suiteContinue(code int) {
	f.mu.Lock()
	chain := f.chain
	done, nk, na := f.nextDone, f.nextKind, f.nextArgs
	next := f.lastNext
	f.mu.Unlock()
	if !chain || !done {
		return
	}
	if code != 0 {
		f.mu.Lock()
		f.chain = false
		f.mu.Unlock()
		f.log("suite : pause — l'étape a échoué (remède dans le journal ci-dessus), corrigez puis Suite")
		return
	}
	f.suiteStep(nk, na, next)
}

func vmLabel(r vmRow) string {
	dot := "○"
	if r.live {
		dot = "●"
	}
	mode := r.mode
	if mode == "" || mode == "inconnu" {
		mode = "?"
	}
	return fmt.Sprintf("%s %s [%s · %s]", dot, r.name, r.hyp, mode)
}

func newFyApp(a fyne.App, exeDir string) *fyApp {
	return &fyApp{
		app:    a,
		logCh:  make(chan string, 2000),
		sel:    -1,
		cfgPth: filepath.Join(exeDir, "kit", "cgo-vm.yaml"),
		cgoExe: filepath.Join(exeDir, "cgo"),
	}
}

func (f *fyApp) buildUI() {
	f.win = f.app.NewWindow("Meteolink Kit — centre de contrôle")
	f.win.Resize(fyne.NewSize(940, 700))

	f.vmList = widget.NewList(
		func() int {
			f.mu.Lock()
			defer f.mu.Unlock()
			return len(f.rows)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if int(i) < len(f.rows) {
				o.(*widget.Label).SetText(vmLabel(f.rows[i]))
			}
		},
	)
	f.vmList.OnSelected = func(id widget.ListItemID) {
		f.mu.Lock()
		f.sel = int(id)
		f.mu.Unlock()
	}

	mkRow := func(defs []buttonDef) *fyne.Container {
		btns := make([]fyne.CanvasObject, 0, len(defs))
		for _, d := range defs {
			id := d.id
			btns = append(btns, widget.NewButton(d.label, func() { f.dispatch(id) }))
		}
		return container.NewHBox(btns...)
	}

	f.lockLbl = widget.NewLabel("Aucune machine verrouillée.")
	f.sshLbl = widget.NewLabel("SSH : —")
	f.userEdit = widget.NewEntry()
	f.userEdit.SetPlaceHolder("utilisateur Ubuntu (ex. fanasina)")
	if u := cfgSSHUser(f.cfgPth); u != "" {
		f.userEdit.SetText(u)
	}
	userRow := container.NewHBox(
		widget.NewLabel("Utilisateur :"),
		f.userEdit,
		widget.NewButton("Sauver", func() { f.dispatch(248) }),
	)
	f.dashLbl = widget.NewLabel("Dashboard : —")
	f.journal = widget.NewMultiLineEntry()
	f.journal.Disable()
	f.journal.Wrapping = fyne.TextWrapWord
	f.jScroll = container.NewScroll(f.journal)
	f.status = widget.NewLabel("Prêt.")
	f.nextLbl = widget.NewLabel("→ Prochaine : …")

	left := container.NewVBox(
		widget.NewLabel("Machine virtuelle (VirtualBox / VMware) :"),
		container.NewVScroll(f.vmList),
		mkRow(groupActions),
		f.lockLbl,
		widget.NewLabel("Accès SSH — diagnostiquer, clé, boot"),
		f.sshLbl,
		userRow,
		mkRow(accessActions),
		widget.NewLabel("Déployer"),
		f.dashLbl,
		mkRow(deployActions),
	)
	right := container.NewVBox(
		widget.NewLabel("Contrôle"),
		mkRow(controlActions[:4]),
		mkRow(controlActions[4:8]),
		mkRow(controlActions[8:10]),
		mkRow(controlActions[10:]),
		widget.NewLabel("Journal"),
		f.jScroll,
		f.status,
		f.nextLbl,
	)
	split := container.NewHSplit(left, right)
	split.Offset = 0.52
	f.win.SetContent(split)
}

func main() {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		fmt.Fprintln(os.Stderr, "pas d'écran (ni DISPLAY ni WAYLAND_DISPLAY) — ici : ./cgo kit tui")
		os.Exit(2)
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "introuvable : "+err.Error())
		os.Exit(1)
	}
	f := newFyApp(app.New(), filepath.Dir(exe))
	f.buildUI()
	go f.pump()
	go f.scan()
	f.refreshStatus()
	f.win.ShowAndRun()
}
