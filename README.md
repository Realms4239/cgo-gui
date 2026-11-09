# cgo-gui — Meteolink Kit (vue native)

GUI native de Meteolink : **Windows (Win32 brut, pur Go)** et **Linux
(Fyne, `linux && cgo`)**. Les deux vues pilotent les mêmes actions :
chaque bouton devient un verbe `cgo kit …` du binaire voisin, jamais un
import du monolithe.

- Module : `github.com/Realms4239/cgo-gui` (`go 1.25`)
- Binaire voisin : `cgo`/`cgo.exe` **à côté** de `cgo-gui.exe`
  (ou `$CGO_BIN`, ou le `PATH`). La distribution livre le couple.

## Pré-requis

| Vue | Pré-requis |
| --- | --- |
| Windows | aucun (pur Go, `golang.org/x/sys`) |
| Linux (Fyne) | GCC, X11/OpenGL, `pkg-config`, headers Wayland/X11 (`libgl1-mesa-dev`, `libglfw3-dev`, `xorg-dev`, `libwayland-dev`, `pkg-config`, `libxkbcommon-dev` — voir `docs/` du monolithe CGO ; si absents, la vue refuse poliment vers `cgo kit tui`) |

## Vérification

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...   # stub seul (CI headless, croisé)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./... # vue Windows
CGO_ENABLED=1 go build ./...                           # vue Linux Fyne (toolchain + écran)
go test ./...
```

## Structure

- `core.go` — cœur partagé sans tag (actions, journal, sondes).
- Fichiers `windows` (Win32 brut, `LockOSThread` obligatoire).
- `fygui_linux.go` — vue Linux Fyne (`//go:build linux && cgo`), refus
  poli sans écran (`cgo kit tui`).
- `kit/` — client fin : verbes délégués au binaire voisin (`cgo kit …`),
  config yaml locale, `SSH`, `Next` (checklist), helpers yaml/NAT.
- `vm/` — détection/pilotage hyperviseurs (VMware VMrun, VirtualBox
  VBoxManage), aucune dépendance externe.
- `stub.go` (`//go:build !windows && (!linux || !cgo)`) — redirection
  honnête + sortie 2 ; `go vet`/`go build` en `CGO_ENABLED=0` restent
  verts (l'échec d'origine `GOOS=linux CGO_ENABLED=0` est corrigé par
  cette garde : sans elle, Fyne exigeait le CGO).

## Version

SemVer indépendant : départ `v0.1.0` (`0.y.z`, API non garantie).
`kit.KitVersion` doit concorder avec `cgo version` du binaire voisin
(source unique : le CLI).
