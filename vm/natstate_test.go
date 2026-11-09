package vm

import "testing"

// Détection d'état partagée (rapport 1.3.4 B1) : la pose NAT a échoué sur
// une VM allumée lue « éteinte » — ces garde-fous purs verrouillent le
// vocabulaire d'erreurs et la bascule chaud/froid.

func TestMentionsExists(t *testing.T) {
	for _, out := range []string{
		`VBoxManage: error: A NAT rule named 'cgo-ssh' already exists`,
		`rule already in use by another VM`,
	} {
		if !mentionsExists(out) {
			t.Errorf("mentionsExists(%q) = false", out)
		}
	}
	if mentionsExists("VBoxManage: error: Invalid machine state") {
		t.Error("mentionsExists sur erreur d'état = true (faux bénin)")
	}
}

func TestMentionsLocked(t *testing.T) {
	for _, out := range []string{
		`VBoxManage: error: The machine is locked by a session`,
		`VBOX_E_INVALID_OBJECT_STATE`,
		`E_FAIL (0x80004005)`,
	} {
		if !mentionsLocked(out) {
			t.Errorf("mentionsLocked(%q) = false", out)
		}
	}
	if mentionsLocked("rule already exists") {
		t.Error("mentionsLocked sur already-exists = true (mauvaise bascule)")
	}
}

func TestMentionsNotRunning(t *testing.T) {
	for _, out := range []string{
		`VBoxManage: error: Machine 'x' is not currently running`,
		`Could not find a running machine`,
	} {
		if !mentionsNotRunning(out) {
			t.Errorf("mentionsNotRunning(%q) = false", out)
		}
	}
}

func TestHotColdLabels(t *testing.T) {
	if hotCold(true) == hotCold(false) {
		t.Error("libellés chaud/froid identiques — l'erreur ne dirait plus la cause")
	}
	if hotHint(true) == "" || hotHint(false) == "" {
		t.Error("remède vide — l'erreur retomberait au « exit status 1 » nu")
	}
}

func TestQuotedName(t *testing.T) {
	if got := quotedName(`"ubuntu26 04" {1234-abcd}`); got != "ubuntu26 04" {
		t.Errorf("quotedName = %q", got)
	}
	if got := quotedName(`ubuntu sans quotes`); got != "" {
		t.Errorf("ligne sans quotes = %q (attendu vide)", got)
	}
}
