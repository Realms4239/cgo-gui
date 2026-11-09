// Runner — même surface que le Runner du CLI (internal/kit), implémentée
// en déléguant au binaire kit voisin (`cgo kit <verbe> …`, même dispatch,
// mêmes codes). Les flux (Stdout/Stderr) restent ceux posés par la vue.
package kit

import (
	"io"
	"os"
	"path/filepath"
)

// Runner — client fin des verbes kit.
type Runner struct {
	Stdout io.Writer
	Stderr io.Writer
	Root   string // racine du dépôt (pour hostTunCmd) : dossier du binaire
}

// NewRunner — Root = dossier du binaire kit voisin (défaut pris en
// charge ci-dessous ; l'app GUI le force en fait à côté de cgo).
func NewRunner() *Runner {
	r := &Runner{Stdout: os.Stdout, Stderr: os.Stderr}
	if b, err := Bin(); err == nil && b != "" {
		r.Root = filepath.Dir(b)
	} else if ex, err := os.Executable(); err == nil {
		r.Root = filepath.Dir(ex)
	} else {
		r.Root = "."
	}
	return r
}

// run — alias interne : args du verbe après `--config`.
func (r *Runner) run(cfgPath string, args ...string) int {
	return runVerb(r.Stderr, cfgPath, args...)
}

// cfgPath — chemin du yaml machine-local : `<dir kit>/kit/cgo-vm.yaml`
// (même aligement que a.cfgPath = join(dir(cgo), "kit", "cgo-vm.yaml")).
func (r *Runner) cfgPath() string {
	if r.Root != "" {
		p := filepath.Join(r.Root, "kit", "cgo-vm.yaml")
		if fileOK(p) {
			return p
		}
	}
	return filepath.Join(r.Root, "kit", "cgo-vm.yaml")
}

// ── Verbes employés par les deux vues ──────────────────────────────────

// DNS — meteolink.dev → IP VM dans le hosts local.
func (r *Runner) DNS(c *Config) int { return r.run(r.cfgPath(), "dns") }

// TLS — confiance du certificat dashboard.
func (r *Runner) TLS(c *Config) int { return r.run(r.cfgPath(), "tls") }

// Ensure — SSH up, sinon boot VM + attente + IP auto-découverte.
func (r *Runner) Ensure(c *Config, cfgPath string, deep bool) int {
	return runVerbDeep(r.Stderr, cfgPath, deep, []string{"ensure"})
}

// Deploy — build + ensure + cross-compile + scp + install + health.
func (r *Runner) Deploy(c *Config, cfgPath string, deep bool) int {
	return runVerbDeep(r.Stderr, cfgPath, deep, []string{"deploy"})
}

// Svc — pilote dashboard distant (start|stop|restart|status).
func (r *Runner) Svc(c *Config, sub string) int {
	return runVerb(r.Stderr, r.cfgPath(), "svc", sub)
}

// Testbed — banc de mesure invité (check|up|down).
func (r *Runner) Testbed(c *Config, rest []string) int {
	args := append([]string{"testbed"}, rest...)
	return runVerb(r.Stderr, r.cfgPath(), args...)
}

// Backup — rapatrie les runs gelés (tar.gz horodaté).
func (r *Runner) Backup(c *Config, dest string) int {
	return runVerb(r.Stderr, r.cfgPath(), "backup", dest)
}

// Snapshot — point de restauration VM (garde-fou).
func (r *Runner) Snapshot(c *Config, name string, deep bool) int {
	args := []string{"snapshot"}
	if name != "" {
		args = append(args, name)
	}
	return runVerbDeep(r.Stderr, r.cfgPath(), deep, args)
}

// Netinfo — réseau invité en lecture seule.
func (r *Runner) Netinfo(c *Config) int { return runVerb(r.Stderr, r.cfgPath(), "netinfo") }

// Nic — mode réseau NIC1 (affichage ou nat|bridged|hostonly).
func (r *Runner) Nic(c *Config, cfgPath string, rest []string, deep bool) int {
	args := append([]string{"nic"}, rest...)
	return runVerbDeep(r.Stderr, cfgPath, deep, args)
}

// Guest — prépare l'invité (--check = audit seul).
func (r *Runner) Guest(c *Config, cfgPath string, check bool) int {
	args := []string{"guest"}
	if check {
		args = append(args, "--check")
	}
	return runVerb(r.Stderr, r.cfgPath(), args...)
}

// Vnet — réseau HÔTE vers la VM.
func (r *Runner) Vnet(c *Config) int { return runVerb(r.Stderr, r.cfgPath(), "vnet") }

// Logs — tail du journal serveur VM.
func (r *Runner) Logs(c *Config, n int) int { return runVerb(r.Stderr, r.cfgPath(), "logs") }

// Verify — empreintes SHA-256 des archives.
func (r *Runner) Verify(c *Config) int { return runVerb(r.Stderr, r.cfgPath(), "verify") }

// ShipCheck — porte d'embarquement release.
func (r *Runner) ShipCheck(c *Config) int { return runVerb(r.Stderr, r.cfgPath(), "shipcheck") }
