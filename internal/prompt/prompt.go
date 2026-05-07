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

// Sentinel return values for Choose / ChooseEx.
const (
	ChoiceExit = -1 // user typed 'x' / 'X'
	ChoiceBack = -2 // user typed 'b' / 'B'
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
		fmt.Println("Invalid integer, try again.")
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

// Choose is a thin shim that calls ChooseEx without back/exit shortcuts.
// Kept for legacy call-sites that don't need navigation letters.
func Choose(question string, options []string) int {
	return ChooseEx(question, options, false, false)
}

// ChooseEx renders a numbered menu plus an optional 'x' shortcut. A single
// letter covers both navigation cases: at the top level 'x' returns
// ChoiceExit, in a submenu it returns ChoiceBack. The two never overlap in
// the same prompt (top-level menus don't have a parent, submenus aren't
// the program's exit point), so one key is unambiguous and the user only
// has to learn one binding.
func ChooseEx(question string, options []string, withBack, withExit bool) int {
	for {
		fmt.Println()
		fmt.Println(question)
		for i, o := range options {
			fmt.Printf("  %d) %s\n", i+1, o)
		}
		switch {
		case withBack:
			fmt.Println("  x) back")
		case withExit:
			fmt.Println("  x) exit")
		}
		a := strings.ToLower(strings.TrimSpace(Ask("Choose", "")))
		if a == "" {
			continue
		}
		if a == "x" || a == "back" || a == "exit" || a == "quit" || a == "q" {
			if withBack {
				return ChoiceBack
			}
			if withExit {
				return ChoiceExit
			}
		}
		n, err := strconv.Atoi(a)
		if err == nil && n >= 1 && n <= len(options) {
			return n
		}
		fmt.Println("Invalid option.")
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
			fmt.Println("Empty password, try again.")
			continue
		}

		fmt.Print("Confirm password: ")
		_ = exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
		pw2 := read()
		_ = exec.Command("stty", "-F", "/dev/tty", "echo").Run()
		fmt.Println()

		if pw1 != pw2 {
			fmt.Println("Passwords don't match, try again.")
			continue
		}
		return pw1
	}
}

// Pause blocks until Enter is pressed (used between menu actions).
func Pause(msg string) {
	if msg == "" {
		msg = "Press Enter to continue"
	}
	fmt.Printf("\n%s... ", msg)
	read()
}

// AskIPOrCIDR prompts repeatedly until a valid IP or CIDR is entered. Empty cancels (returns "").
func AskIPOrCIDR(question string) string {
	for {
		a := strings.TrimSpace(Ask(question+" (empty cancels)", ""))
		if a == "" {
			return ""
		}
		if net.ParseIP(a) != nil {
			return a
		}
		if _, _, err := net.ParseCIDR(a); err == nil {
			return a
		}
		fmt.Println("Invalid IP/CIDR (e.g. 1.2.3.4 or 10.0.0.0/24).")
	}
}
