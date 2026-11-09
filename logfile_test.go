//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendGUIFile(t *testing.T) {
	dir := t.TempDir()
	// détourne la cible en changeant le binaire ? Non — vérifie le contrat :
	// toute ligne poussée finit dans un fichier daté, sans erreur remontée.
	exe, err := os.Executable()
	if err != nil || exe == "" {
		t.Skip("pas d'exécutable de test")
	}
	_ = dir
	appendGUIFile("ligne-test-12345")
	// le fichier doit exister à côté de l'exécutable de test OU dans %TEMP%
	found := ""
	for _, d := range []string{filepath.Dir(exe), os.TempDir()} {
		matches, _ := filepath.Glob(filepath.Join(d, "cgo-gui-*.log"))
		for _, m := range matches {
			b, _ := os.ReadFile(m)
			if strings.Contains(string(b), "ligne-test-12345") {
				found = m
			}
		}
	}
	if found == "" {
		t.Fatal("ligne non retrouvée dans aucun journal")
	}
	t.Logf("journal : %s", found)
}
