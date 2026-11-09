//go:build !windows && (!linux || !cgo)

// Stub sans CGO (ou autre GOOS) : pas de Fyne, pas de GL. Même refus
// honnête que la vue Linux sans écran, puis sortie 2.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "pas de vue GUI pour ce système — ici : cgo kit tui")
	os.Exit(2)
}
