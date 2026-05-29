package docker

import (
	"fmt"
	"strings"

	"erp24/internal/runner"
)

// conflicting is the set of distro-shipped or alternative docker stacks that
// must not coexist with docker-ce. We deliberately do NOT include the
// docker-ce* packages here — re-running the docker module on an already-
// installed server would otherwise tear down a working engine before
// reinstalling it, with the autoremove cascade potentially deleting state.
var conflicting = []string{
	"docker.io", "docker-doc", "docker-compose",
	"podman", "podman-compose",
	"runc", // distro-shipped runc clashes with containerd.io's bundled one
}

func RemoveConflicts() error {
	if runner.HasCommand("docker") {
		runner.Log("docker engine present — skipping conflict-removal sweep.")
		return nil
	}
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
	return runner.Run("apt-get", "install", "-y", "-qq",
		"docker-ce", "docker-ce-cli", "containerd.io",
		"docker-buildx-plugin", "docker-compose-plugin")
}

// EnableDaemon enables + starts the rootful docker daemon. Idempotent —
// re-running on a healthy host is a no-op for systemd.
func EnableDaemon() error {
	return runner.Run("systemctl", "enable", "--now", "docker.service")
}
