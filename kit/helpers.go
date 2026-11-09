// Helpers config (yaml plat) + NAT + Next : la veille s'exécute côté CLI,
// la GUI n'a que les lectures/écritures locales (mêmes fichiers, mêmes clés).
package kit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Realms4239/cgo-gui/vm"
)

// writeFileAtomic — écriture via fichier temporaire + rename (même
// sémantique que le helper du CLI : jamais de yaml tronqué en cours).
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".cgo-gui-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// saveConfigValue — clé plate au-dessus du yaml (fidèle au CLI).
func saveConfigValue(path, key, val string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			b = []byte{}
		} else {
			return err
		}
	}
	lines := []string{}
	for _, ln := range strings.Split(string(b), "\n") {
		stripped := strings.Split(ln, "#")[0]
		trim := strings.TrimSpace(stripped)
		if trim == "" {
			continue
		}
		if !strings.HasPrefix(trim, " ") && !strings.HasPrefix(trim, "\t") {
			k := strings.TrimSpace(trim[:minInt(len(trim), indexColon(trim))])
			if k == key {
				lines = append(lines, key+": \""+val+"\"")
				goto found
			}
		}
		lines = append(lines, ln)
	}
	lines = append(lines, key+": \""+val+"\"")
found:
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")), 0644)
}

func indexColon(s string) int {
	if i := strings.Index(s, ":"); i >= 0 {
		return i
	}
	return len(s)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// setYAMLKey — clé sous section (ex. ssh: user/host/port/key).
func setYAMLKey(path, key, val string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			b = []byte{}
		} else {
			return err
		}
	}
	lines := []string{}
	found := false
	for _, ln := range strings.Split(string(b), "\n") {
		stripped := strings.Split(ln, "#")[0]
		trim := strings.TrimSpace(stripped)
		if trim == "" {
			continue
		}
		if !found && trim == key+":" {
			lines = append(lines, "  "+key+":")
			continue
		}
		if !found && strings.HasPrefix(trim, key+":") {
			indent := ln[:len(ln)-len(strings.TrimLeft(ln, " \t"))]
			lines = append(lines, indent+key+": \""+val+"\"")
			found = true
			continue
		}
		lines = append(lines, ln)
	}
	if !found {
		lines = append(lines, "  "+key+": \""+val+"\"")
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")), 0644)
}

// SaveVMX — persiste vm_name/vmx_path/hypervisor (Clean avant écriture,
// comme au CLI).
func SaveVMX(path, vmx, hypervisor string) error {
	vmx = filepath.Clean(vmx)
	name := strings.TrimSuffix(filepath.Base(vmx), filepath.Ext(vmx))
	var first error
	set := func(key, val string) {
		if err := saveConfigValue(path, key, val); err != nil && first == nil {
			first = err
		}
	}
	if strings.HasSuffix(strings.ToLower(vmx), ".vbox") {
		set("vbox_path", vmx)
	} else {
		set("vmx_path", vmx)
	}
	set("vm_name", name)
	if hypervisor != "" {
		set("hypervisor", hypervisor)
	}
	return first
}

// SaveSSHTarget — mémorise user/host/port/key dans la section ssh:
// (valeurs vides ignorées pour les edits partiels).
func SaveSSHTarget(path, user, host, port, key string) error {
	for k, v := range map[string]string{"user": user, "host": host, "port": port, "key": key} {
		if v == "" {
			continue
		}
		if err := setYAMLKey(path, k, v); err != nil {
			return err
		}
	}
	return nil
}

// ── NAT / verrou ─────────────────────────────────────────────────────────

// keepExplicitTarget — cible loopback explicite de l'opérateur.
func keepExplicitTarget(host string) bool {
	h := strings.TrimSpace(host)
	return strings.HasPrefix(h, "127.") || h == "localhost" || h == "::1" ||
		strings.HasPrefix(h, "[::1]")
}

// natHostPort — port hôte du forward NAT (config ou 2222).
func natHostPort(c *Config) string {
	if strings.TrimSpace(c.NatHostPort) != "" {
		return strings.TrimSpace(c.NatHostPort)
	}
	return "2222"
}

// isVBoxGuestSpace — 10.0.2.x, fait topologique.
func isVBoxGuestSpace(host string) bool {
	return strings.HasPrefix(strings.TrimSpace(host), "10.0.2.")
}

// NATForwardTarget — (host, port, true) quand le cas NAT s'applique.
// Mêmes règles que le CLI (jamais de mutation).
func NATForwardTarget(c *Config, host string) (string, string, bool) {
	h := strings.TrimSpace(host)
	if h == "" || h == "auto" || keepExplicitTarget(h) {
		return "", "", false
	}
	if isVBoxGuestSpace(h) {
		return "127.0.0.1", natHostPort(c), true
	}
	if c.VMPath() == "" {
		return "", "", false
	}
	hyp, err := selectDriver(vm.Detect(), c.Hypervisor, c.VMPath())
	if err != nil {
		return "", "", false
	}
	nf, ok := hyp.(vm.NatForwarder)
	_ = nf
	if !ok || hyp.NetMode(c.VMPath()) != "nat" {
		return "", "", false
	}
	return nf.NatHostAddr(), natHostPort(c), true
}

// selectDriver — pilote pour un chemin (fidèle au CLI).
var errNoDriver = errors.New("pilote hyperviseur absent pour le chemin de VM")

func selectDriver(hs []vm.Hypervisor, cfgHyp, path string) (vm.Hypervisor, error) {
	want := vm.HypForPath(path)
	for _, h := range hs {
		if h.Name() == want {
			return h, nil
		}
	}
	if cfgHyp != "" && cfgHyp != "auto" {
		for _, h := range hs {
			if h.Name() == cfgHyp {
				return h, nil
			}
		}
	}
	if len(hs) == 1 {
		return hs[0], nil
	}
	return nil, errNoDriver
}

// AutoLockSingle — verrouillage auto si une seule VM (même contrat que le
// CLI : message honnête si 0 ou N VMs).
func AutoLockSingle(c *Config, cfgPath string) (string, bool) {
	if p := c.VMPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return "VM déjà verrouillée", true
		}
	}
	vms := vm.ScanVMs(false)
	switch len(vms) {
	case 0:
		return "aucune VM trouvée — créez/importez-en une, puis Rescanner", false
	case 1:
	default:
		if p := vm.Pick(vms, c.VMName); p != "" {
			vms = []string{p}
		} else {
			return vmPickMsg(vms), false
		}
	}
	path := vms[0]
	hyp := vm.HypForPath(path)
	if hyp == "" {
		hyp = c.Hypervisor
	}
	if err := SaveVMX(cfgPath, path, hyp); err != nil {
		return "verrouillage : " + err.Error(), false
	}
	return "verrouillée : " + filepath.Base(path) + " — Suite pour continuer", true
}

func vmPickMsg(vms []string) string {
	return "Plusieurs VMs trouvées — verrouillez-en une dans la liste, puis Suite"
}

// runCapture — verbe en capture (pour Readme).
func runCapture(args ...string) string {
	bin, err := Bin()
	if err != nil {
		return ""
	}
	cmd := exec.Command(bin, append([]string{"kit"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// Readme — texte du guide (subprocess `cgo kit readme`, verbe du CLI).
func Readme() string {
	return runCapture("readme")
}
