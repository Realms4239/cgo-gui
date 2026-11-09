// Parité avec internal/kit/vnet.go (canonique, non exporté) : isVBoxGuestSpace
// est miroiré dans core.go — si l'un des deux dérive, ce test casse.
// Garder la table alignée sur TestIsVBoxGuestSpace côté kit.
package main

import "testing"

func TestIsVBoxGuestSpaceParity(t *testing.T) {
	for in, want := range map[string]bool{
		"10.0.2.15":       true,
		" 10.0.2.1 ":      true,
		"10.0.2.1":        true,
		"192.168.174.128": false,
		"127.0.0.1":       false,
		"10.0.3.5":        false,
		"":                false,
		"meteolink.dev":   false,
	} {
		if got := isVBoxGuestSpace(in); got != want {
			t.Errorf("isVBoxGuestSpace(%q) = %v, want %v (miroir kit/vnet.go)", in, got, want)
		}
	}
}
