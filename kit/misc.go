// Sondes réseau courtes (tcp, ifaces, dns) + SSH client-side : stdlib seule.
package kit

import (
	"context"
	"net"
	"strings"
	"time"
)

// lookupHostFast — résolution bornée (ne pend pas sur le DNS externe).
func lookupHostFast(name string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return net.DefaultResolver.LookupHost(ctx, name)
}

// portOpen — dial TCP court : « sshd absent » de « VM éteinte » sans ssh.
func portOpen(host, port string) bool {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// hostIfaceOn — nom d'une interface UP non-loopback portant une IP du /24.
func hostIfaceOn(sub string) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, it := range ifaces {
		if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := it.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			if strings.HasPrefix(ip.String(), sub) {
				return it.Name
			}
		}
	}
	return ""
}

// shq — citation shell POSIX d'un argument.
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
