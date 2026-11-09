// Next — une copie fidèle du moteur de prochaine-étape (internal/kit/next.go).
// Pur (lecture locale + sondes ssh/tcp/https) : pas de subprocess `next`.
package kit

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Realms4239/cgo-gui/vm"
)

// NextStep — une étape de la checklist (même contrat que le CLI).
type NextStep struct {
	ID     string // vm, vnet, boot, cible, port, cle, binaire, svc, dns, tls, banc
	Label  string // texte banner (« Prochaine : … »)
	State  string // "ok", "ko", "attente" (amont non vert)
	Detail string // preuve courte
	Remedy string // commande CLI exacte
	Verb   string // verbe buttonAction GUI ("" = guidance seule)
	Args   []string
}

// GatherNextVer — la checklist, mêmes paliers, même ordre que le CLI.
func GatherNextVer(c *Config, wantVer string) []NextStep {
	steps := []NextStep{}
	blocked := false
	push := func(s NextStep) {
		if blocked && s.State == "ok" {
			s.State = "attente"
			s.Detail = "en attente (" + steps[len(steps)-1].ID + " d'abord)"
		}
		if s.State != "ok" {
			blocked = true
		}
		steps = append(steps, s)
	}

	var hyp vm.Hypervisor
	locked, hypName, vmx := "", "", ""
	vpath := c.VMPath()
	switch {
	case vpath == "":
		push(NextStep{ID: "vm", Label: "Verrouiller une VM", State: "ko",
			Detail: "aucune VM verrouillée",
			Remedy: "Suite : scan + verrouillage auto (1 seule) — cgo kit scan",
			Verb:   "bg:scan"})
	default:
		st, err := os.Stat(vpath)
		if err != nil || st.IsDir() {
			push(NextStep{ID: "vm", Label: "VM introuvable", State: "ko",
				Detail: vpath + " n'existe plus",
				Remedy: "Suite : re-scan + verrouillage auto — cgo kit scan",
				Verb:   "bg:scan"})
			break
		}
		h, err := selectDriver(vm.Detect(), c.Hypervisor, vpath)
		if err != nil {
			want := vm.HypForPath(vpath)
			push(NextStep{ID: "vm", Label: "Pilote manquant", State: "ko",
				Detail: fmt.Sprintf("%s : pilote %s absent — installez %s", filepath.Base(vpath), want,
					map[string]string{"virtualbox": "VirtualBox", "vmware": "VMware Workstation"}[want]),
				Remedy: "installez " + map[string]string{"virtualbox": "VirtualBox", "vmware": "VMware Workstation"}[want]})
			break
		}
		locked, hypName, hyp, vmx = filepath.Base(vpath), h.Name(), h, vpath
		push(NextStep{ID: "vm", Label: "VM verrouillée", State: "ok",
			Detail: fmt.Sprintf("%s (%s)", locked, hypName)})
	}

	host := strings.TrimSpace(c.SSHHost)
	sub := ""
	if parts := strings.Split(host, "."); len(parts) == 4 {
		if ip := net.ParseIP(host); ip != nil && ip.To4() != nil && ip.IsPrivate() &&
			!strings.HasPrefix(host, "127.") && host != "localhost" {
			sub = strings.Join(parts[:3], ".") + "."
		} else if strings.HasPrefix(host, "127.") || host == "localhost" {
			sub = "local"
		}
	}
	natH, natP, isNAT := NATForwardTarget(c, host)
	switch {
	case isNAT && portOpen(natH, natP):
		push(NextStep{ID: "vnet", Label: "Réseau hôte", State: "ok", Detail: "forward local " + natH + ":" + natP + " en écoute"})
	case isNAT:
		push(NextStep{ID: "vnet", Label: "Forward NAT fermé", State: "ko",
			Detail: natH + ":" + natP + " muet (invitée " + host + " injoignable en direct — normal)",
			Remedy: "cgo kit ensure", Verb: "bg:ensure"})
	case host == "" || host == "auto":
		push(NextStep{ID: "vnet", Label: "Réseau hôte", State: "attente", Detail: "cible auto",
			Remedy: "cgo kit ensure (découvre la cible)", Verb: "bg:ensure"})
	case sub == "local":
		push(NextStep{ID: "vnet", Label: "Réseau hôte", State: "ok", Detail: "forward local"})
	case sub == "":
		push(NextStep{ID: "vnet", Label: "Réseau hôte", State: "ok", Detail: "cible publique"})
	case hostIfaceOn(sub) != "":
		push(NextStep{ID: "vnet", Label: "Réseau hôte", State: "ok", Detail: "hôte sur " + sub + "0/24"})
	default:
		push(NextStep{ID: "vnet", Label: "Réseau hôte tombé", State: "ko",
			Detail:  "aucune interface sur " + sub + "0/24",
			Remedy:  "cgo kit vnet", Verb: "bg:vnet"})
	}

	effHost, effPort := host, sshPort(c)
	if isNAT {
		effHost, effPort = natH, natP
	}

	if hyp == nil {
		push(NextStep{ID: "boot", Label: "VM allumée", State: "attente", Detail: "pas de VM"})
	} else if vmIsRunning(hyp, vmx) {
		push(NextStep{ID: "boot", Label: "VM allumée", State: "ok", Detail: "en cours d'exécution"})
	} else {
		push(NextStep{ID: "boot", Label: "Démarrer la VM", State: "ko",
			Detail: "éteinte", Remedy: "cgo kit ensure", Verb: "bg:vmon"})
	}

	if host == "" || host == "auto" {
		push(NextStep{ID: "cible", Label: "Cible SSH", State: "attente",
			Detail: "auto — résolue par la prochaine étape", Remedy: "cgo kit ensure", Verb: "bg:ensure"})
	} else {
		push(NextStep{ID: "cible", Label: "Cible SSH", State: "ok", Detail: effHost + ":" + effPort})
	}

	if !stepOK(steps, "cible") {
		push(NextStep{ID: "port", Label: "Port SSH", State: "attente", Detail: "cible d'abord"})
	} else if portOpen(effHost, effPort) {
		push(NextStep{ID: "port", Label: "Port SSH", State: "ok", Detail: effHost + ":" + effPort + " ouvert"})
	} else {
		push(NextStep{ID: "port", Label: "Port SSH fermé", State: "ko",
			Detail:  effHost + ":" + effPort + " muet",
			Remedy:  "cgo kit ensure", Verb: "bg:ensure"})
	}

	if !stepOK(steps, "port") {
		push(NextStep{ID: "cle", Label: "Clé SSH", State: "attente", Detail: "port d'abord"})
	} else if _, err := sshProbeAs(c, effHost, effPort); err == nil {
		push(NextStep{ID: "cle", Label: "Clé SSH", State: "ok", Detail: "acceptée par " + c.SSHUser + "@" + host})
	} else {
		push(NextStep{ID: "cle", Label: "Poser la clé SSH", State: "ko",
			Detail: "refusée par l'invité (mot de passe une fois)",
			Remedy: "cgo kit keysetup", Verb: "console:keysetup", Args: []string{"keysetup"}})
	}

	proj := c.ProjectDir
	if proj == "" && c.SSHUser != "" {
		proj = "/home/" + c.SSHUser + "/cgo"
	}
	if !stepOK(steps, "cle") {
		push(NextStep{ID: "binaire", Label: "Binaire déployé", State: "attente", Detail: "clé d'abord"})
	} else if out, err := sshExecAs(c, effHost, effPort, "test -x "+shq(proj+"/cgo-linux")+" && echo OK"); err == nil && strings.Contains(out, "OK") {
		push(NextStep{ID: "binaire", Label: "Binaire déployé", State: "ok", Detail: proj + "/cgo-linux"})
	} else {
		push(NextStep{ID: "binaire", Label: "Déployer le binaire", State: "ko",
			Detail: "cgo-linux absent de l'invité",
			Remedy: "cgo kit deploy", Verb: "bg:deploy"})
	}

	if !stepOK(steps, "binaire") {
		push(NextStep{ID: "svc", Label: "Dashboard", State: "attente", Detail: "binaire d'abord"})
	} else if ver, ok := dashProbe(dashProbeURL(c)); ok {
		if dashStale(ver, wantVer) {
			push(NextStep{ID: "svc", Label: "Dashboard périmé", State: "ko",
				Detail: "distant " + ver + ", attendu " + wantVer,
				Remedy: "cgo kit deploy", Verb: "bg:deploy"})
		} else if dashAhead(ver, wantVer) {
			push(NextStep{ID: "svc", Label: "Kit local périmé", State: "ko",
				Detail: "distant " + ver + " plus neuf qu'attendu " + wantVer + " (ne pas downgrader)",
				Remedy: "update local : git pull + rebuild (jamais `cgo kit deploy` ici — downgrade)",
				Verb:   ""})
		} else {
			push(NextStep{ID: "svc", Label: "Dashboard", State: "ok", Detail: "sain (version " + ver + ")"})
		}
	} else if httpOnlyUp(c) {
		push(NextStep{ID: "svc", Label: "Dashboard pré-TLS", State: "ko",
			Detail: "HTTP seul (binaire ≤1.2.2) — svc start ne soignera jamais",
			Remedy: "cgo kit deploy", Verb: "bg:deploy"})
	} else {
		push(NextStep{ID: "svc", Label: "Démarrer le dashboard", State: "ko",
			Detail: "aucune réponse saine",
			Remedy: "cgo kit svc start", Verb: "bg:svc", Args: []string{"start"}})
	}

	dashName := c.DashHost
	if dashName == "" {
		dashName = "meteolink.dev"
	}
	if !stepOK(steps, "svc") {
		push(NextStep{ID: "dns", Label: "Nom meteolink.dev", State: "attente", Detail: "dashboard d'abord"})
	} else if host == "" || host == "auto" {
		push(NextStep{ID: "dns", Label: "Nom meteolink.dev", State: "attente", Detail: "cible d'abord"})
	} else if isNAT {
		push(NextStep{ID: "dns", Label: "Nom meteolink.dev", State: "attente", Detail: "NAT : dashboard via le forward " + effHost + ":" + effPort + " — le nom ne s'applique pas"})
	} else if dnsMapsTo(dashName, host) {
		push(NextStep{ID: "dns", Label: "Nom meteolink.dev", State: "ok", Detail: dashName + " → " + host})
	} else {
		push(NextStep{ID: "dns", Label: "Mapper meteolink.dev (admin)", State: "ko",
			Detail: dashName + " ne pointe pas vers " + host,
			Remedy: "Suite : mappe le nom (app déjà élevée) — cgo kit dns", Verb: "bg:dns"})
	}

	if !stepOK(steps, "dns") {
		push(NextStep{ID: "tls", Label: "Confiance HTTPS", State: "attente", Detail: "nom d'abord"})
	} else if tlsTrusted(dashURLFor(c)) {
		push(NextStep{ID: "tls", Label: "Confiance HTTPS", State: "ok", Detail: "magasin OS OK"})
	} else {
		push(NextStep{ID: "tls", Label: "Confiance HTTPS (admin)", State: "ko",
			Detail: "certificat non reconnu par le système",
			Remedy: "Suite : installe la confiance (app déjà élevée) — cgo kit tls", Verb: "bg:tls"})
	}

	if !stepOK(steps, "cle") {
		push(NextStep{ID: "banc", Label: "Banc de mesure", State: "attente", Detail: "clé d'abord"})
	} else if out, err := sshExecAs(c, effHost, effPort, "curl -fsS -m3 http://10.200.0.1:8081/small -o /dev/null && timeout 1 bash -c '</dev/tcp/10.200.0.1/5201' && ! sudo -n ip netns list 2>&1 | grep -qi password && echo PLANE_OK"); err == nil && strings.Contains(out, "PLANE_OK") {
		push(NextStep{ID: "banc", Label: "Banc de mesure", State: "ok", Detail: "small + bulk + sudo — campagnes possibles"})
	} else {
		push(NextStep{ID: "banc", Label: "Banc de mesure", State: "ko",
			Detail: "absent — Démarrer gèlerait zéro ligne",
			Remedy: "cgo kit testbed up", Verb: "bg:testbed", Args: []string{"up"}})
	}
	return steps
}

// FirstOpen — premier palier non-vert : LA prochaine étape. Pur et testé.
func FirstOpen(steps []NextStep) *NextStep {
	for i := range steps {
		if steps[i].State != "ok" {
			return &steps[i]
		}
	}
	return nil
}

func stepOK(steps []NextStep, id string) bool {
	for _, s := range steps {
		if s.ID == id {
			return s.State == "ok"
		}
	}
	return false
}

// vmIsRunning — racine commune (chemin OU nom enregistré).
func vmIsRunning(hyp vm.Hypervisor, vmx string) bool {
	return vm.IsRunning(hyp, vmx)
}

func sshPort(c *Config) string {
	if c.SSHPort != "" {
		return c.SSHPort
	}
	return "22"
}

func dashURLFor(c *Config) string {
	host := c.DashHost
	if host == "" {
		host = "meteolink.dev"
	}
	port := c.DashPort
	if port == "" {
		port = "9090"
	}
	return "https://" + host + ":" + port
}

// dashProbeURL — sonde le dashboard sur la CIBLE VIVANTE (IP SSH).
func dashProbeURL(c *Config) string {
	if h := strings.TrimSpace(c.SSHHost); h != "" && h != "auto" {
		port := c.DashPort
		if port == "" {
			port = "9090"
		}
		if natH, _, isNAT := NATForwardTarget(c, h); isNAT {
			return "https://" + natH + ":" + port
		}
		return "https://" + h + ":" + port
	}
	return dashURLFor(c)
}

func dashProbe(url string) (string, bool) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	cl := &http.Client{Timeout: 5 * time.Second, Transport: tr}
	resp, err := cl.Get(url + "/api/health")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var doc struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil || !doc.OK {
		return "", false
	}
	return doc.Version, true
}

func dnsMapsTo(name, ip string) bool {
	if isVBoxGuestSpace(strings.TrimSpace(ip)) {
		return false
	}
	addrs, err := lookupHostFast(name)
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if a == ip {
			return true
		}
	}
	return false
}

func tlsTrusted(url string) bool {
	cl := &http.Client{Timeout: 5 * time.Second}
	resp, err := cl.Get(url + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var doc struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil || !doc.OK {
		return false
	}
	return true
}

// httpOnlyUp — le dashboard répond en HTTP brut mais pas en HTTPS (pré-TLS).
func httpOnlyUp(c *Config) bool {
	port := c.DashPort
	if port == "" {
		port = "9090"
	}
	host := c.SSHHost
	if host == "" || host == "auto" {
		return false
	}
	if natH, _, isNAT := NATForwardTarget(c, host); isNAT {
		host = natH
	}
	cl := &http.Client{Timeout: 3 * time.Second}
	resp, err := cl.Get("http://" + host + ":" + port + "/api/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return err == nil
}

// dashStale — le binaire distant est-il d'une version mineure ANTÉRIEURE ?
func dashStale(have, want string) bool {
	if want == "" || have == "" {
		return false
	}
	mh, nh := verMajMin(have)
	mw, nw := verMajMin(want)
	if mh < 0 || mw < 0 {
		return false
	}
	return mh < mw || (mh == mw && nh < nw)
}

// dashAhead — miroir de dashStale (distant plus neuf).
func dashAhead(have, want string) bool {
	if want == "" || have == "" {
		return false
	}
	mh, nh := verMajMin(have)
	mw, nw := verMajMin(want)
	if mh < 0 || mw < 0 {
		return false
	}
	return mh > mw || (mh == mw && nh > nw)
}

func verMajMin(v string) (int, int) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return -1, -1
	}
	a, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	b, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return -1, -1
	}
	return a, b
}

// muxSocket — socket de multiplexage ssh (non-Windows).
func muxSocket(host, port string) string {
	if host == "" {
		host = "auto"
	}
	if port == "" {
		port = "ssh"
	}
	return filepath.Join(os.TempDir(), "cgo-gui-"+host+"-"+port+".sock")
}
