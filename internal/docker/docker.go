package docker

import (
	"fmt"
	"os/user"
	"strings"

	"lsm/internal/runner"
)

var conflicting = []string{
	"docker.io", "docker-ce", "docker-ce-cli", "containerd.io", "runc",
	"docker-buildx-plugin", "docker-compose-plugin",
	"podman", "podman-compose",
}

func RemoveConflicts() error {
	for _, p := range conflicting {
		runner.TryRun("apt-get", "remove", "-y", p)
	}
	runner.TryRun("apt-get", "autoremove", "-y", "-qq")
	return nil
}

func InstallRepo() error {
	if err := runner.Run("apt-get", "install", "-y", "-qq", "ca-certificates", "curl"); err != nil {
		return err
	}
	if err := runner.Run("install", "-m", "0755", "-d", "/etc/apt/keyrings"); err != nil {
		return err
	}
	if err := runner.Run("curl", "-fsSL",
		"https://download.docker.com/linux/debian/gpg",
		"-o", "/etc/apt/keyrings/docker.asc"); err != nil {
		return err
	}
	if err := runner.Run("chmod", "a+r", "/etc/apt/keyrings/docker.asc"); err != nil {
		return err
	}

	arch, err := runner.Capture("dpkg", "--print-architecture")
	if err != nil {
		return err
	}
	arch = strings.TrimSpace(arch)

	codename, err := runner.Capture("bash", "-c", ". /etc/os-release && echo $VERSION_CODENAME")
	if err != nil {
		return err
	}
	codename = strings.TrimSpace(codename)

	repo := fmt.Sprintf("deb [arch=%s signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian %s stable\n",
		arch, codename)
	return runner.Stdin(repo, "tee", "/etc/apt/sources.list.d/docker.list")
}

func InstallEngine() error {
	if err := runner.Run("apt-get", "update", "-qq"); err != nil {
		return err
	}
	// systemd-container provides `machinectl`, used to run the rootless
	// setup tool inside the rootless user's session.
	return runner.Run("apt-get", "install", "-y", "-qq",
		"docker-ce", "docker-ce-cli", "containerd.io",
		"docker-buildx-plugin", "docker-compose-plugin",
		"docker-ce-rootless-extras", "uidmap", "dbus-user-session",
		"systemd-container")
}

func DisableRootful() {
	runner.TryRun("systemctl", "disable", "--now", "docker.service", "docker.socket")
}

func userExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

func SetupRootlessUser(name string) error {
	if !userExists(name) {
		if err := runner.Run("useradd", "-m", "-s", "/bin/bash", name); err != nil {
			return err
		}
	}

	subuid, _ := runner.Capture("grep", fmt.Sprintf("^%s:", name), "/etc/subuid")
	if strings.TrimSpace(subuid) == "" {
		if err := runner.Run("usermod", "--add-subuids", "100000-165535", name); err != nil {
			return err
		}
	}
	subgid, _ := runner.Capture("grep", fmt.Sprintf("^%s:", name), "/etc/subgid")
	if strings.TrimSpace(subgid) == "" {
		if err := runner.Run("usermod", "--add-subgids", "100000-165535", name); err != nil {
			return err
		}
	}

	if err := runner.Run("loginctl", "enable-linger", name); err != nil {
		return err
	}

	// machinectl pertence ao package systemd-container — pode faltar em
	// instalações Debian minimal. Garante presente antes de chamar.
	if !runner.HasCommand("machinectl") {
		runner.Log("machinectl em falta — a instalar systemd-container.")
		if err := runner.Run("apt-get", "install", "-y", "-qq", "systemd-container"); err != nil {
			return fmt.Errorf("install systemd-container: %w", err)
		}
	}

	return runner.Run("machinectl", "shell", name+"@", "/bin/bash", "-c",
		"dockerd-rootless-setuptool.sh install")
}
