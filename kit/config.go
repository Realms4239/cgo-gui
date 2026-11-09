// Config — même schéma que le yaml cgo-vm.yaml du CLI (module cgo).
// Le parseur lit le même sous-ensemble clé:valeur (plus jamais
// d'ambiguïté env/yaml : les surcharges CGO_* restent appliquées
// après le fichier, comme côté CLI).
package kit

import (
	"errors"
	"os"
	"os/user"
	"strings"
)

// Config — miroir fidèle du schéma cgo-vm.yaml (16 clés + défauts).
type Config struct {
	SSHUser     string
	SSHHost     string
	SSHPort     string
	SSHKey      string
	VMName      string
	VMXPath     string
	VBoxPath    string
	Hypervisor  string
	Snapshot    string
	ProjectDir  string
	DashPort    string
	DashHost    string
	GoMinVer    string
	NatHostPort string
}

// VMPath — LE chemin de la VM verrouillée, quel que soit l'hyperviseur
// (.vmx ou .vbox). Tout le code lit ÇA, jamais les champs bruts.
func (c *Config) VMPath() string {
	if c.VMXPath != "" {
		return c.VMXPath
	}
	return c.VBoxPath
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// KitVersion — version du kit adjacent (source unique : `cgo version`).
// Miroir de la constante du CLI ; bumpée à chaque release avec le tag.
const KitVersion = "1.3.5"

// LoadConfig lit le yaml (défauts + env puis fichier).
// NB : le CLI résout en plus l'IP invitée (GuestIP) ; ici le chargement
// reste du parse seul — la résolution vit côté CLI pour chaque verbe.
func LoadConfig(path string) (*Config, error) {
	c := &Config{
		SSHUser:    env("CGO_SSH_USER", ""),
		SSHHost:    env("CGO_SSH_HOST", "auto"),
		SSHPort:    env("CGO_SSH_PORT", "22"),
		SSHKey:     env("CGO_SSH_KEY", "~/.ssh/id_ed25519"),
		ProjectDir: env("CGO_PROJECT_DIR", ""),
		DashPort:   env("CGO_DASHBOARD_PORT", "9090"),
		DashHost:   env("CGO_DASHBOARD_HOST", "meteolink.dev"),
		GoMinVer:   "1.25",
		Hypervisor: "auto",
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, nil // config absente → défauts + env (portable, pas fatal)
	}
	envHost, envPort, envUser, envKey := c.SSHHost, c.SSHPort, c.SSHUser, c.SSHKey
	envProject, envDash, envDashHost := c.ProjectDir, c.DashPort, c.DashHost
	section := ""
	for _, raw := range strings.Split(string(b), "\n") {
		stripped := strings.Split(raw, "#")[0]
		indented := len(stripped) > 0 && (stripped[0] == ' ' || stripped[0] == '\t')
		ln := strings.TrimSpace(stripped)
		ln = strings.TrimSuffix(ln, "\r")
		if ln == "" {
			continue
		}
		if !indented {
			section = ""
		}
		if strings.HasSuffix(ln, ":") && !strings.Contains(ln, " ") {
			section = strings.TrimSuffix(ln, ":")
			continue
		}
		i := strings.Index(ln, ":")
		if i < 0 {
			continue
		}
		k, v := ln[:i], strings.TrimSpace(ln[i+1:])
		k = strings.TrimSpace(k)
		v = strings.Trim(v, `"'`)
		full := k
		if indented && section != "" {
			full = section + "_" + k
		}
		switch full {
		case "ssh_user":
			c.SSHUser = v
		case "ssh_host":
			c.SSHHost = v
		case "ssh_port":
			c.SSHPort = v
		case "ssh_key":
			c.SSHKey = v
		case "vm_name":
			c.VMName = v
		case "vmx_path":
			c.VMXPath = v
		case "vbox_path":
			c.VBoxPath = v
		case "hypervisor":
			c.Hypervisor = v
		case "snapshot":
			c.Snapshot = v
		case "project_dir":
			c.ProjectDir = v
		case "dashboard_port":
			c.DashPort = v
		case "dashboard_host":
			c.DashHost = v
		case "go_min_version":
			c.GoMinVer = v
		case "nat_host_port":
			c.NatHostPort = v
		}
	}
	// L'environnement gagne (contrat CLI : CGO_SSH_HOST force la cible).
	if envHost != "" {
		c.SSHHost = envHost
	}
	if envPort != "" {
		c.SSHPort = envPort
	}
	if envUser != "" {
		c.SSHUser = envUser
	}
	if envKey != "" {
		c.SSHKey = envKey
	}
	if envProject != "" {
		c.ProjectDir = envProject
	}
	if envDash != "" {
		c.DashPort = envDash
	}
	if envDashHost != "" {
		c.DashHost = envDashHost
	}
	_ = envHost
	_ = envPort
	_ = envUser
	_ = envKey
	_ = envProject
	_ = envDash
	_ = envDashHost
	return c, nil
}

// LoadConfigFast — alias (pas de résolution côté client non plus).
func LoadConfigFast(path string) (*Config, error) {
	return LoadConfig(path)
}

var errNoSSHUser = errors.New("ssh_user vide — renseignez-le (GUI : champ Utilisateur + Sauver, ou cgo kit keysetup --user …)")

// userHome — $HOME, sauf sous sudo où ~/ désigne l'utilisateur d'origine.
func userHome() string {
	if r := os.Geteuid(); r == 0 {
		if su := os.Getenv("SUDO_USER"); su != "" {
			if u, err := user.Lookup(su); err == nil && u.HomeDir != "" {
				return u.HomeDir
			}
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}
