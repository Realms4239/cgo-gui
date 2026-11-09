package vm

import (
	"os"
	"strings"
	"testing"
)

// Preuve bout-en-bout fichier : garde + backup + écriture + idempotence,
// sur .vmx factice (jamais la VM du banc).
func TestVmwareSetNetModeFile(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/t.vmx"
	os.WriteFile(p, []byte("memsize = \"4096\"\nethernet0.present = \"TRUE\"\nethernet0.connectionType = \"hostonly\"\n"), 0644)
	v := &vmware{}
	if err := v.SetNetMode(p, "nat"); err != nil {
		t.Fatalf("set: %v", err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), `"nat"`) || strings.Contains(string(raw), "hostonly") {
		t.Fatalf("contenu:\n%s", raw)
	}
	if _, err := os.Stat(p + ".meteolink-bak"); err != nil {
		t.Fatal("backup manquant")
	}
	// re-set identique : backup d'origine conservé, contenu stable
	before, _ := os.ReadFile(p + ".meteolink-bak")
	v.SetNetMode(p, "nat")
	after, _ := os.ReadFile(p + ".meteolink-bak")
	if string(before) != string(after) {
		t.Fatal("backup écrasé au 2e passage")
	}
}
