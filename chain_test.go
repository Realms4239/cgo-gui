//go:build windows

package main

import (
	"strings"
	"testing"
)

func lastLog(a *app) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.logText) == 0 {
		return ""
	}
	return a.logText[len(a.logText)-1]
}

func TestSuiteStepGreen(t *testing.T) {
	a := &app{chain: true}
	a.suiteStep("", nil, "tout est vert — dashboard prêt")
	if a.chain {
		t.Fatal("chaîne non arrêtée sur vert")
	}
	if !strings.Contains(lastLog(a), "tout est vert") {
		t.Fatalf("journal = %q", lastLog(a))
	}
}

func TestSuiteStepUnknownVerb(t *testing.T) {
	a := &app{chain: true}
	a.suiteStep("bogus:verb", nil, "installez VirtualBox")
	if a.chain {
		t.Fatal("chaîne non arrêtée sur verbe inconnu")
	}
	if g := lastLog(a); !strings.Contains(g, "pas d'action automatique") || !strings.Contains(g, "installez VirtualBox") {
		t.Fatalf("journal = %q", g)
	}
}

func TestKnownSuiteVerbs(t *testing.T) {
	for _, v := range []string{"bg:scan", "bg:ensure", "bg:deploy", "bg:dns", "bg:tls", "bg:testbed", "bg:svc", "console:keysetup", "direct:lock", "direct:suite"} {
		if !isKnownSuiteVerb(v) {
			t.Errorf("%s devrait être connu", v)
		}
	}
	for _, v := range []string{"", "bogus", "bg:nope"} {
		if isKnownSuiteVerb(v) {
			t.Errorf("%s ne devrait PAS être connu", v)
		}
	}
}
