package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Realms4239/cgo-gui/kit"
	"github.com/Realms4239/cgo-gui/vm"
)

// Exhaustivité : chaque bouton câblé a une action, chaque action existe.
func TestGUIButtonMap(t *testing.T) {
	seen := map[string]int{}
	for _, group := range [][]buttonDef{groupActions, accessActions, deployActions, controlActions} {
		for _, b := range group {
			kind, _ := buttonAction(b.id)
			if kind == "" {
				t.Errorf("bouton %d %q sans action", b.id, b.label)
			}
			seen[kind]++
		}
	}
	// actions critiques présentes
	for _, want := range []string{"bg:scan", "bg:deploy", "bg:diag", "bg:mkkey", "console:keysetup", "bg:ensure", "bg:svc", "bg:nic-toggle", "bg:nic"} {
		found := false
		for _, group := range [][]buttonDef{groupActions, accessActions, deployActions, controlActions} {
			for _, b := range group {
				if k, _ := buttonAction(b.id); k == want {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("action %q non câblée à un bouton", want)
		}
	}
	// libellés non vides, ids uniques
	ids := map[int]string{}
	for _, group := range [][]buttonDef{groupActions, accessActions, deployActions, controlActions} {
		for _, b := range group {
			if strings.TrimSpace(b.label) == "" {
				t.Errorf("bouton %d sans libellé", b.id)
			}
			if prev, ok := ids[b.id]; ok {
				t.Errorf("id %d dupliqué (%q vs %q)", b.id, prev, b.label)
			}
			ids[b.id] = b.label
		}
	}
}

// Actions console (mot de passe / admin) : jamais en fond silencieux.
// keysetup seul garde sa console (mot de passe TTY) ; dns/tls sont
// non-interactifs et tournent en fond journalisé depuis l'élévation unique.
func TestGUIConsoleActions(t *testing.T) {
	for _, id := range []int{213} {
		kind, _ := buttonAction(id)
		if !strings.HasPrefix(kind, "console:") {
			t.Errorf("bouton %d : %q devrait être console:", id, kind)
		}
	}
}

// Nouveaux boutons intégration scripts : mappés, jamais muets.
func TestGUIScriptButtons(t *testing.T) {
	for id, want := range map[int]string{245: "bg:hosttun", 246: "bg:guest", 247: "bg:vnet", 248: "direct:saveuser", 249: "bg:guide", 250: "direct:suite"} {
		kind, _ := buttonAction(id)
		if kind != want {
			t.Errorf("bouton %d : %q, voulu %q", id, kind, want)
		}
	}
}

// TestDblClickBypassesButtonMap — #13 : la liste des VM (101, littéral :
// idVMList est Windows-only, ce test compile aussi sous Linux) n'a PAS de
// verbe — wndProc envoie son double-clic DIRECT à lockSelected (même
// handler que le bouton Verrouiller), jamais via onButton (vide = le
// double-clic passait pour mort).
func TestDblClickBypassesButtonMap(t *testing.T) {
	if kind, _ := buttonAction(101); kind != "" {
		t.Fatalf("liste 101 : %q — le dbl-clic doit rester hors buttonAction (wndProc direct)", kind)
	}
}

// cfgSSHUser — lecture pure du yaml, sans hyperviseur ni réseau.
func TestCfgSSHUser(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/cgo-vm.yaml"
	yml := "ssh:\n  user: fanasina\n  host: auto\n  port: 22\nvm_name: \"ubuntu Fanasina\"\n"
	if err := os.WriteFile(p, []byte(yml), 0644); err != nil {
		t.Fatal(err)
	}
	if got := cfgSSHUser(p); got != "fanasina" {
		t.Fatalf("got %q, want fanasina", got)
	}
	if got := cfgSSHUser(dir + "/absent.yaml"); got != "" {
		t.Fatalf("manquant → vide, got %q", got)
	}
}

// fakeHyp — pilote factice : seul Name() compte pour hypForDrivers.
type fakeHyp struct{ name string }

func (f *fakeHyp) Name() string                      { return f.name }
func (f *fakeHyp) Exe() string                       { return "" }
func (f *fakeHyp) Running() []string                 { return nil }
func (f *fakeHyp) Start(vmx string) error            { return nil }
func (f *fakeHyp) StartGUI(vmx string) error         { return nil }
func (f *fakeHyp) Stop(vmx string) error             { return nil }
func (f *fakeHyp) GuestIP(vmx string) string         { return "" }
func (f *fakeHyp) NetMode(vmx string) string         { return "" }
func (f *fakeHyp) SetNetMode(vmx, mode string) error { return nil }

// hypForDrivers — l'extension gagne sur l'ordre de détection ET sur le
// nom configuré : un .vbox ne part jamais chez vmrun (bug racine GUI).
func TestHypForDrivers(t *testing.T) {
	vw := &fakeHyp{name: "vmware"}
	vb := &fakeHyp{name: "virtualbox"}
	hs := []vm.Hypervisor{vw, vb} // vmware DÉTECTÉ EN PREMIER (pire cas)
	mkCfg := func(path, hyp string) *kit.Config {
		return &kit.Config{VMXPath: path, VBoxPath: "", Hypervisor: hyp}
	}
	if h := hypForDrivers(hs, mkCfg(`D:\VMs\ubuntu Fanasina\ubuntu Fanasina.vbox`, "vmware")); h == nil || h.Name() != "virtualbox" {
		t.Fatalf(".vbox + detect[vmware..] + config vmware → virtualbox, got %v", h)
	}
	if h := hypForDrivers(hs, mkCfg(`D:\ubuntu.vmx`, "virtualbox")); h == nil || h.Name() != "vmware" {
		t.Fatalf(".vmx + config virtualbox → vmware, got %v", h)
	}
	if h := hypForDrivers(hs, mkCfg("", "")); h == nil || h.Name() != "vmware" {
		t.Fatalf("sans chemin → premier détecté, got %v", h)
	}
	if h := hypForDrivers(nil, mkCfg(`D:\x.vbox`, "")); h != nil {
		t.Fatalf("aucun pilote → nil, got %v", h)
	}
	if h := hypForDrivers([]vm.Hypervisor{vb}, mkCfg(`D:\x.vbox`, "")); h == nil || h.Name() != "virtualbox" {
		t.Fatalf("seul vbox détecté → virtualbox, got %v", h)
	}
}

// shortDiag — le sentinel utilisateur vide a son libellé (pas « voir journal »).
func TestShortDiagSSHUser(t *testing.T) {
	if got := shortDiag("ssh_user vide"); got != "utilisateur vide" {
		t.Fatalf("got %q", got)
	}
	if got := shortDiag("dial tcp: connection refused"); got != "port 22 fermé" {
		t.Fatalf("regression refused : %q", got)
	}
}

// TestAllVerbsHandled — régression incident v1.2.12 (bouton 204 bg:nic
// mappé mais sans case dans les switches : clic muet). Chaque verbe que
// buttonAction peut retourner doit avoir un `case kind == "verbe":` dans
// Win32 runKind (actions.go) ET Fyne dispatchKind (fygui_linux.go).
func TestAllVerbsHandled(t *testing.T) {
	verbs := map[string]bool{}
	for id := 0; id < 400; id++ {
		if kind, _ := buttonAction(id); kind != "" {
			verbs[kind] = true
		}
	}
	if len(verbs) == 0 {
		t.Fatal("aucun verbe collecté — buttonAction injoignable ?")
	}
	win, err := os.ReadFile("actions.go")
	if err != nil {
		t.Fatalf("lecture actions.go : %v", err)
	}
	fy, err := os.ReadFile("fygui_linux.go")
	if err != nil {
		t.Fatalf("lecture fygui_linux.go : %v", err)
	}
	for verb := range verbs {
		needle := `case kind == "` + verb + `":`
		if !strings.Contains(string(win), needle) {
			t.Errorf("verbe %q sans case dans Win32 runKind (actions.go)", verb)
		}
		if !strings.Contains(string(fy), needle) {
			t.Errorf("verbe %q sans case dans Fyne dispatchKind (fygui_linux.go)", verb)
		}
	}
}
