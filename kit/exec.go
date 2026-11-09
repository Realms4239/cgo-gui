// Exécution des verbes `cgo kit …` : le dispatch, l'erreur et les
// sorties vivent côté CLI ; ici on stream et on renvoie le code.
package kit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// binOnce — le binaire kit adjacent, résolu une fois.
var (
	binOnce sync.Once
	binPath string
	binErr  error
)

// Bin — binaire kit voisin : $CGO_BIN, sinon cgo[.exe] à côté de l'exe GUI,
// sinon le PATH. La distribution livre cgo-gui.exe + cgo.exe côte à côte.
func Bin() (string, error) {
	binOnce.Do(func() {
		if b := os.Getenv("CGO_BIN"); b != "" {
			binPath, binErr = filepath.Abs(b)
			return
		}
		if ex, err := os.Executable(); err == nil {
			dir := filepath.Dir(ex)
			for _, name := range []string{cgoName(), "cgo"} {
				if p := filepath.Join(dir, name); fileOK(p) {
					binPath = p
					return
				}
			}
		}
		if p, err := exec.LookPath("cgo"); err == nil {
			binPath = p
			return
		}
		binErr = errors.New("cgo introuvable — dézippez l'archive complète (cgo + cgo-gui côte à côte)")
	})
	return binPath, binErr
}

func fileOK(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func cgoName() string {
	if runtime.GOOS == "windows" {
		return "cgo.exe"
	}
	return "cgo"
}

// runVerb — un verbe kit avec ses args (config d'abord, œil flags).
// Sortie streamée vers outErr (journal GUI), code cohérent avec le CLI.
func runVerb(outErr io.Writer, cfgPath string, args ...string) int {
	bin, err := Bin()
	if err != nil {
		fmt.Fprintln(outErr, err.Error())
		return 2
	}
	full := []string{bin, "kit", "--config", cfgPath}
	full = append(full, args...)
	fmt.Fprintf(outErr, "$ %s kit %s\n", bin, strings.Join(args, " "))
	cmd := exec.Command(full[0], full[1:]...)
	cmd.Stdout = outErr
	cmd.Stderr = outErr
	cmd.Stdin = strings.NewReader("")
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(interface{ ExitCode() int }); ok {
			return ws.ExitCode()
		}
		if n, err2 := strconv.Atoi(strings.TrimPrefix(ee.String(), "exit status ")); err2 == nil {
			return n
		}
	}
	return 0
}

// runVerbDeep — verbe + --deep.
func runVerbDeep(outErr io.Writer, cfgPath string, deep bool, args []string) int {
	if !deep {
		args = append(args, "--shallow")
	}
	return runVerb(outErr, cfgPath, args...)
}

// BgCmd — processus de fond (contexte d'ouverture de console).
func BgCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// BgCmdCtx — même chose avec contexte.
func BgCmdCtx(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
