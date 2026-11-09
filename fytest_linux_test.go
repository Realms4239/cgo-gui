//go:build linux

// Vue Fyne pilotée sans écran (driver test) : chaque bouton réagit,
// la garde anti-double-clic tient, le journal reçoit. Preuve headless
// que l'équivalent Linux fait le même travail que le Win32.
package main

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/Realms4239/cgo-gui/kit"
)

func newTestFy(t *testing.T) *fyApp {
	t.Helper()
	a := test.NewApp()
	f := newFyApp(a, t.TempDir())
	f.buildUI()
	return f
}

func drainLog(f *fyApp) []string {
	var out []string
	for {
		select {
		case ln := <-f.logCh:
			out = append(out, ln)
		default:
			return out
		}
	}
}

func TestFyneLockSansSelection(t *testing.T) {
	f := newTestFy(t)
	f.dispatch(202) // Verrouiller, liste vide : refus explicite
	lines := drainLog(f)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "liste vide") {
		t.Fatalf("attendu le rappel de liste vide, journal : %q", joined)
	}
}

func TestFyneGardeAntiDoubleClic(t *testing.T) {
	f := newTestFy(t)
	release := make(chan struct{})
	f.runBg("lent", func(r *kit.Runner) int {
		<-release
		return 0
	})
	f.runBg("second", func(r *kit.Runner) int { return 0 })
	lines := drainLog(f)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "patience") {
		t.Fatalf("2e action non rejetée, journal : %q", joined)
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.mu.Lock()
		busy := f.busy
		f.mu.Unlock()
		if busy == "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFyneVerrouilleLigne(t *testing.T) {
	f := newTestFy(t)
	f.mu.Lock()
	f.rows = []vmRow{{path: "/tmp/fausse.vmx", name: "fausse", hyp: "vmware", mode: "nat"}}
	f.sel = 0
	f.mu.Unlock()
	f.dispatch(202)
	lines := drainLog(f)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "verrouillée : fausse") {
		t.Fatalf("verrouillage non journalisé : %q", joined)
	}
	c, _ := kit.LoadConfig(f.cfgPth)
	if c.VMXPath != "/tmp/fausse.vmx" {
		t.Fatalf("config non écrite : %q", c.VMXPath)
	}
}

func TestFyneVmLabel(t *testing.T) {
	if got := vmLabel(vmRow{name: "u", hyp: "virtualbox", mode: "nat"}); !strings.Contains(got, "○ u [virtualbox · nat]") {
		t.Fatalf("label éteinte : %q", got)
	}
	if got := vmLabel(vmRow{name: "u", hyp: "?", mode: "inconnu", live: true}); !strings.Contains(got, "● u [? · ?]") {
		t.Fatalf("label allumée : %q", got)
	}
}
