package vm

import (
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestParseRunningVMs(t *testing.T) {
	out := "Total running VMs: 2\r\n\"Ubuntu 24.04\" {a1b2c3d4-e5f6-7890-abcd-ef1234567890}\r\n\"srv\" {11111111-2222-3333-4444-555555555555}\r\n"
	got := parseRunningVMs(out)
	if len(got) != 2 || got[0] != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Fatalf("got %q", got)
	}
	if got := parseRunningVMs("Total running VMs: 0\r\n"); len(got) != 0 {
		t.Fatalf("vide attendu, got %q", got)
	}
}

func TestSameVM(t *testing.T) {
	if !SameVM(`D:\VMs\ubu\ubu.vbox`, "d:/vms/ubu/ubu.vbox") {
		t.Fatal("même VM non reconnue (casse/séparateurs)")
	}
	if SameVM(`D:\VMs\ubu\ubu.vbox`, `D:\VMs\autre\autre.vbox`) {
		t.Fatal("faux positif")
	}
}

func TestHypForPath(t *testing.T) {
	for p, want := range map[string]string{
		`D:\VMs\ubuntu Fanasina\ubuntu Fanasina.vbox`: "virtualbox",
		`/home/u/VirtualBox VMs/x.vbox`:              "virtualbox",
		`D:\ubuntu.vmx`:                              "vmware",
		`./rel.vmx`:                                  "vmware",
		`D:\x.VBOX`:                                  "virtualbox",
		`D:\x.VMX`:                                   "vmware",
		`D:\sans-ext`:                                "",
		`D:\x.vmdk`:                                  "",
	} {
		if got := HypForPath(p); got != want {
			t.Errorf("HypForPath(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestNormKeyDedup(t *testing.T) {
	a := normKey(`D:\VMs\Ubu\ubu.vbox`)
	b := normKey(`d:/vms/ubu/ubu.vbox`)
	c := normKey(`D:\VMs\Ubu\ubu.vbox`)
	if a != b || a != c {
		t.Fatalf("clés %q %q %q — même fichier, clés différentes", a, b, c)
	}
	if normKey(`D:\a\ubu.vbox`) == normKey(`D:\b\ubu.vbox`) {
		t.Fatal("faux positif entre dossiers")
	}
}

func TestBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("pas de socket locale")
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !busyPort(strconv.Itoa(port)) {
		t.Fatal("port occupé non détecté")
	}
	if busyPort("1") {
		t.Fatal("port 1 détecté occupé (droits ?)")
	}
}

// NetMode se lit dans le fichier, sans hyperviseur : ponté/nat/hôte-only.
// NOTE — runProgramInGuest/copyFile PROUVÉS INOPÉRANTS sur open-vm-tools 13
// + Workstation 1x (VIX_E_FILE_NOT_FOUND, exit 0 vide) : aucun transport
// invité par ce canal, l'apt+ssh reste la voie. Ne pas réintroduire sans
// banc apparié qui le prouve.
func TestNetModeVmx(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		`ethernet0.connectionType = "bridged"`:  "ponté",
		`ethernet0.connectionType = "nat"`:      "nat",
		`ethernet0.connectionType = "hostonly"`: "hôte-only",
		`ethernet0.present = "TRUE"`:            "inconnu",
	}
	v := &vmware{}
	i := 0
	for content, want := range cases {
		i++
		p := dir + "/t.vmx"
		os.WriteFile(p, []byte(content+"\n"), 0644)
		if got := v.NetMode(p); got != want {
			t.Errorf("cas %d: got %q want %q", i, got, want)
		}
	}
	if got := v.NetMode(dir + "/absent.vmx"); got != "inconnu" {
		t.Errorf("fichier absent: got %q", got)
	}
}

func TestApplyVmxNetMode(t *testing.T) {
	orig := "ethernet0.present = \"TRUE\"\nethernet0.connectionType = \"hostonly\"\nethernet0.virtualDev = \"vmxnet3\"\n"
	got := applyVmxNetMode(orig, "nat")
	if !strings.Contains(got, `ethernet0.connectionType = "nat"`) {
		t.Fatalf("non remplacé:\n%s", got)
	}
	if strings.Contains(got, "hostonly") {
		t.Fatalf("ancien mode resté:\n%s", got)
	}
	// ordre et autres lignes intacts
	if !strings.Contains(got, `ethernet0.virtualDev = "vmxnet3"`) {
		t.Fatalf("voisin perdu:\n%s", got)
	}
	// absent → ajouté après le bloc ethernet0
	got2 := applyVmxNetMode("memsize = \"4096\"\nethernet0.present = \"TRUE\"\n", "bridged")
	if !strings.Contains(got2, `ethernet0.connectionType = "bridged"`) {
		t.Fatalf("non ajouté:\n%s", got2)
	}
	// idempotent
	if again := applyVmxNetMode(got, "nat"); again != got {
		t.Fatalf("non idempotent:\n%s", again)
	}
}
