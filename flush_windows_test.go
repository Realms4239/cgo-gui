//go:build windows

// Comptabilité du flush append-only : shown suit, trim géré, HWND nul
// toléré (SendMessageW(NULL) rend 0 — pas de fenêtre en test).
package main

import "testing"

func TestFlushAccounting(t *testing.T) {
	a := &app{}
	for i := 0; i < 5; i++ {
		a.appendLog("l")
	}
	a.flushLog()
	if a.shown != 5 {
		t.Fatalf("shown=%d, voulu 5", a.shown)
	}
	for i := 0; i < 400; i++ {
		a.appendLog("l")
	}
	if len(a.logText) != 300 {
		t.Fatalf("mémoire=%d, voulu 300", len(a.logText))
	}
	a.flushLog()
	if a.shown > 300 || a.shown <= 5 {
		t.Fatalf("shown=%d après flood (attendu 6..300)", a.shown)
	}
	for i := 0; i < 10 && a.logDirty; i++ {
		a.flushLog()
	}
	if a.logDirty {
		t.Fatal("dirty coincé à vrai après vidage")
	}
	if a.shown != len(a.logText) {
		t.Fatalf("shown=%d, mémoire=%d — divergent", a.shown, len(a.logText))
	}
}
