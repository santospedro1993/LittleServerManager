package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var DryRun bool

func ts() string { return time.Now().Format("15:04:05") }

func Log(format string, args ...any) {
	fmt.Printf("[%s] %s\n", ts(), fmt.Sprintf(format, args...))
}

func Section(name string) {
	fmt.Println()
	fmt.Println("=== " + name + " ===")
}

func RequireRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root (use sudo)")
	}
	return nil
}

func HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Run executes cmd, streaming stdout/stderr.
func Run(name string, args ...string) error {
	line := name + " " + strings.Join(args, " ")
	if DryRun {
		Log("DRY: %s", line)
		return nil
	}
	Log("RUN: %s", line)
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// Capture executes cmd, returning combined output. Errors swallowed by caller if needed.
func Capture(name string, args ...string) (string, error) {
	if DryRun {
		Log("DRY-CAPTURE: %s %s", name, strings.Join(args, " "))
		return "", nil
	}
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// Stdin executes cmd, piping `in` to stdin.
func Stdin(in, name string, args ...string) error {
	if DryRun {
		Log("DRY: %s %s (stdin %d bytes)", name, strings.Join(args, " "), len(in))
		return nil
	}
	Log("RUN: %s %s", name, strings.Join(args, " "))
	c := exec.Command(name, args...)
	c.Stdin = strings.NewReader(in)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// TryRun runs cmd but ignores errors (e.g. "remove pkg that may not exist").
func TryRun(name string, args ...string) {
	_ = Run(name, args...)
}

// aptLockTimeout (segundos) é quanto o apt espera pelos locks do dpkg/apt
// antes de desistir. Num Debian acabado de arrancar, o apt-daily /
// unattended-upgrades corre sozinho e prende /var/lib/dpkg/lock-frontend;
// sem isto, as nossas chamadas apt morriam logo com "Could not get lock ...
// exit status 100". 300s deixa o trabalho de fundo terminar primeiro.
// Requer apt >= 1.9.11 (Debian 11+).
const aptLockTimeout = "300"

// aptEnv mantém o apt totalmente não-interativo e impede o diálogo do
// needrestart ("Which services should be restarted?"). Antes só o módulo
// sysupdate aplicava isto; agora todo o apt do erp24 passa por aqui.
var aptEnv = map[string]string{
	"DEBIAN_FRONTEND":          "noninteractive",
	"NEEDRESTART_MODE":         "l",
	"NEEDRESTART_SUSPEND":      "1",
	"APT_LISTCHANGES_FRONTEND": "none",
}

// AptGet corre apt-get com espera-por-lock e ambiente não-interativo, para
// que um apt-daily/unattended-upgrades a decorrer não nos faça falhar. Todas
// as operações apt dos módulos devem passar por aqui.
func AptGet(args ...string) error {
	full := append([]string{"-o", "DPkg::Lock::Timeout=" + aptLockTimeout}, args...)
	return RunEnv(aptEnv, "apt-get", full...)
}

// TryAptGet é o AptGet mas ignora o erro (ex.: remover um pacote que pode
// não estar instalado).
func TryAptGet(args ...string) {
	_ = AptGet(args...)
}

// RunEnv is like Run but sets extra env vars on top of the inherited environment.
func RunEnv(env map[string]string, name string, args ...string) error {
	line := name + " " + strings.Join(args, " ")
	if DryRun {
		Log("DRY: %s (env: %v)", line, env)
		return nil
	}
	Log("RUN: %s", line)
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = os.Environ()
	for k, v := range env {
		c.Env = append(c.Env, k+"="+v)
	}
	return c.Run()
}
