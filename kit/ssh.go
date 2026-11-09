// Client SSH direct (le même binaire `ssh` qu'au poste opérateur) :
// le CLI verbe-side le fait déjà pour les verbes ; la GUI en a besoin
// pour ses sondes (SSH "true" avant d'agir, dashURL, Next).
package kit

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// sshCmd — même argv que le client du CLI (BatchMode, timeout, clé).
func (c *Config) sshCmd() []string {
	key := c.SSHKey
	if strings.HasPrefix(key, "~/") {
		if h := userHome(); h != "" {
			key = filepath.Join(h, key[2:])
		}
	}
	var mux []string
	if runtime.GOOS != "windows" {
		mux = []string{"-o", "ControlMaster=auto", "-o", "ControlPath=" + muxSocket(c.SSHHost, c.SSHPort), "-o", "ControlPersist=30"}
	}
	return append([]string{"ssh",
		"-o", "ConnectTimeout=4", "-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=6",
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", c.SSHPort, "-i", key},
		append(mux, c.SSHUser+"@"+c.SSHHost)...)
}

// SSH — commande distante, sortie combinée, stdin nul.
func (c *Config) SSH(cmdLine string) (string, error) {
	if strings.TrimSpace(c.SSHUser) == "" {
		return "ssh_user vide", errNoSSHUser
	}
	args := append(c.sshCmd(), cmdLine)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// sshExecAs — commande distante sur la cible EFFECTIVE (NAT : forward).
func sshExecAs(c *Config, host, port, cmd string) (string, error) {
	probe := *c
	probe.SSHHost, probe.SSHPort = host, port
	return probe.SSH(cmd)
}

// sshProbeAs — sonde SSH en surchargeant la cible.
func sshProbeAs(c *Config, host, port string) (string, error) {
	return sshExecAs(c, host, port, "true")
}
