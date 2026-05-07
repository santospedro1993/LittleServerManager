package prompt

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var stdin = bufio.NewReader(os.Stdin)

func read() string {
	line, err := stdin.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}

// Ask prompts with a question. If def is non-empty, shown as default and used on empty input.
func Ask(question, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", question, def)
	} else {
		fmt.Printf("%s: ", question)
	}
	a := read()
	if a == "" {
		return def
	}
	return a
}

// AskInt prompts for an integer, retrying until valid.
func AskInt(question string, def int) int {
	for {
		a := Ask(question, strconv.Itoa(def))
		n, err := strconv.Atoi(a)
		if err == nil {
			return n
		}
		fmt.Println("Inteiro inválido, tenta de novo.")
	}
}

// Confirm asks Y/n. defYes controls default on empty input.
func Confirm(question string, defYes bool) bool {
	suf := "[Y/n]"
	if !defYes {
		suf = "[y/N]"
	}
	fmt.Printf("%s %s: ", question, suf)
	a := strings.ToLower(strings.TrimSpace(read()))
	if a == "" {
		return defYes
	}
	return a == "y" || a == "yes" || a == "s" || a == "sim"
}

// Choose displays a numbered menu and returns the chosen 1-based index.
func Choose(question string, options []string) int {
	for {
		fmt.Println()
		fmt.Println(question)
		for i, o := range options {
			fmt.Printf("  %d) %s\n", i+1, o)
		}
		a := strings.TrimSpace(Ask("Escolhe", ""))
		if a == "" {
			continue
		}
		n, err := strconv.Atoi(a)
		if err == nil && n >= 1 && n <= len(options) {
			return n
		}
		fmt.Println("Opção inválida.")
	}
}

// AskPassword reads a password from stdin with terminal echo disabled.
// Asks twice to detect typos. Falls back to echoed input if /dev/tty isn't available.
func AskPassword(question string) string {
	for {
		fmt.Printf("%s: ", question)
		_ = exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
		pw1 := read()
		_ = exec.Command("stty", "-F", "/dev/tty", "echo").Run()
		fmt.Println()
		if pw1 == "" {
			fmt.Println("Password vazia, tenta de novo.")
			continue
		}

		fmt.Print("Confirma password: ")
		_ = exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
		pw2 := read()
		_ = exec.Command("stty", "-F", "/dev/tty", "echo").Run()
		fmt.Println()

		if pw1 != pw2 {
			fmt.Println("Passwords não batem certo, tenta de novo.")
			continue
		}
		return pw1
	}
}

// Pause blocks until Enter is pressed (used between menu actions).
func Pause(msg string) {
	if msg == "" {
		msg = "Pressiona Enter para continuar"
	}
	fmt.Printf("\n%s... ", msg)
	read()
}

// AskIPOrCIDR prompts repeatedly until a valid IP or CIDR is entered. Empty cancels (returns "").
func AskIPOrCIDR(question string) string {
	for {
		a := strings.TrimSpace(Ask(question+" (vazio cancela)", ""))
		if a == "" {
			return ""
		}
		if net.ParseIP(a) != nil {
			return a
		}
		if _, _, err := net.ParseCIDR(a); err == nil {
			return a
		}
		fmt.Println("IP/CIDR inválido (ex: 1.2.3.4 ou 10.0.0.0/24).")
	}
}
