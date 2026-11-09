// Package kit — client fin vers cgo/cgo-kit : AUCUN import du monolithe.
//
// Les deux vues (Win32, Fyne) pilotent les mêmes actions : un bouton =
// un verbe `cgo kit …` exécuté en sous-processus (même dispatch, mêmes
// codes de retour que le CLI), puis le verbe vit côté `cgo`, jamais ici.
// Le seul état local durable est le yaml cgo-vm.yaml (lus/écrits par ce
// paquetage, mêmes clés que le CLI — contrat inchangé).
//
// Géré localement (le CLI ne l'expose pas comme verbe) : chargement du
// yaml, cibles SSH (exec ssh), verrouillage/Next (lecture locale + sondes
// du même paquetage `vm`) et les petits helpers d'écriture de config.
package kit
