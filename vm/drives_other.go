//go:build !windows

package vm

// fixedDrive — hors Windows, pas de distinction accessible en stdlib :
// tout est scanné (les montages réseau restent possibles, cf. --shallow).
func fixedDrive(d string) bool { return true }
