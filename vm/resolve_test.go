package vm

import (
	"testing"
)

// Rapport 1.3.1 : VM 'ubuntu Fanasina' (espace, hors dossier défaut)
// enregistrée + allumée → resolveName("") → registervm → exit 1
// « enregistrement impossible », TOUTE la chaîne NAT tombait.
func TestResolveFromListSpacedName(t *testing.T) {
	list := `"ubuntu Fanasina" {12345678-1234-1234-1234-1234567890ab}
"other box" {aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee}
`
	cfgOf := func(nm string) string {
		if nm == "ubuntu Fanasina" {
			// CfgFile réel, dossier NON défaut (le cas qui tuait tout).
			return "d:/vms/perso/ubuntu fanasina.vbox"
		}
		return "c:/vms/other box.vbox"
	}
	// 1) chemin exact du yaml → trouvé malgré l'espace.
	if got := resolveFromList("d:/vms/perso/ubuntu fanasina.vbox", "ubuntu Fanasina", list, cfgOf); got != "ubuntu Fanasina" {
		t.Fatalf("exact = %q", got)
	}
	// 2) chemin yaml DÉRIVÉ (dossier déplacé) → repli NOM, pas "".
	// (Sans ce repli : registervm sur VM déjà enregistrée → exit 1.)
	if got := resolveFromList("e:/ailleurs/ubuntu fanasina.vbox", "ubuntu Fanasina", list, cfgOf); got != "ubuntu Fanasina" {
		t.Fatalf("drift = %q, want repli nom", got)
	}
	// 3) entrée renommée dans le manager → trouvée par CfgFile.
	list2 := "\"fanasina-prod\" {12345678-1234-1234-1234-1234567890ab}\n"
	cfgOf2 := func(nm string) string {
		if nm == "fanasina-prod" {
			return "d:/vms/ubuntu fanasina.vbox"
		}
		return ""
	}
	if got := resolveFromList("d:/vms/ubuntu fanasina.vbox", "ubuntu Fanasina", list2, cfgOf2); got != "fanasina-prod" {
		t.Fatalf("renommée = %q", got)
	}
	// 4) vraiment absente → "" (registervm garde son sens).
	if got := resolveFromList("d:/vms/fantome.vbox", "fantome", list, cfgOf); got != "" {
		t.Fatalf("absente = %q, want \"\"", got)
	}
}

func TestQuotedNameSpaced(t *testing.T) {
	if got := quotedName(`"ubuntu Fanasina" {12345678-1234-1234-1234-1234567890ab}`); got != "ubuntu Fanasina" {
		t.Fatalf("quotedName = %q", got)
	}
}
