package docker

import (
	"fmt"
	"os/user"
	"strings"

	"lsm/internal/runner"
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
	// systemd-container provides `machinectl`, used to run the rootless
	// setup tool inside the rootless user's session.
	// slirp4netns + fuse-overlayfs are runtime deps for rootless that
	// docker-ce-rootless-extras stopped pulling explicitly on Debian 13.
	return runner.Run("apt-get", "install", "-y", "-qq",
		"docker-ce", "docker-ce-cli", "containerd.io",
		"docker-buildx-plugin", "docker-compose-plugin",
		"docker-ce-rootless-extras", "uidmap", "dbus-user-session",
		"systemd-container", "slirp4netns", "fuse-overlayfs")
}

func DisableRootful() {
	runner.TryRun("systemctl", "disable", "--now", "docker.service", "docker.socket")
}

func UserExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

// SetupRootlessUser provisions the rootless docker user. If the user does
// not yet exist, password must be non-empty — it's set via chpasswd and
// then immediately discarded (lsm never persists it). Re-running on an
// existing user keeps whatever password is already set; pass "" in that
// case.
func SetupRootlessUser(name, password string) error {
	if !UserExists(name) {
		if password == "" {
			return fmt.Errorf("empty password creating user '%s'", name)
		}
		if err := runner.Run("useradd", "-m", "-s", "/bin/bash", name); err != nil {
			return fmt.Errorf("useradd '%s' failed: %w (no changes made — pick another name or fix the underlying issue and retry)", name, err)
		}
		if err := runner.Stdin(fmt.Sprintf("%s:%s", name, password), "chpasswd"); err != nil {
			// Same rollback policy as internal/ssh.CreateUser: don't leave a
			// passwordless account behind.
			runner.Log("WARNING: chpasswd failed for '%s' — rolling back useradd.", name)
			if rbErr := runner.Run("userdel", "-r", name); rbErr != nil {
				return fmt.Errorf("chpasswd failed (%w) AND rollback userdel failed (%v) — MANUAL CLEANUP NEEDED: `userdel -r %s`", err, rbErr, name)
			}
			return fmt.Errorf("set password for '%s': %w (user rolled back; retry)", name, err)
		}
		runner.Log("User '%s' created (rootless docker).", name)
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

	// machinectl ships with systemd-container — may be absent on minimal
	// Debian. Install before we shell into the rootless user's session.
	if !runner.HasCommand("machinectl") {
		runner.Log("machinectl missing — installing systemd-container.")
		if err := runner.Run("apt-get", "install", "-y", "-qq", "systemd-container"); err != nil {
			return fmt.Errorf("install systemd-container: %w", err)
		}
	}
	// slirp4netns / fuse-overlayfs: rootless runtime deps that
	// docker-ce-rootless-extras stopped pulling on Debian 13. Without
	// these dockerd-rootless.sh aborts with "One of slirp4netns ...
	// needs to be installed".
	if !runner.HasCommand("slirp4netns") {
		runner.Log("slirp4netns missing — installing.")
		if err := runner.Run("apt-get", "install", "-y", "-qq", "slirp4netns"); err != nil {
			return fmt.Errorf("install slirp4netns: %w", err)
		}
	}
	if !runner.HasCommand("fuse-overlayfs") {
		runner.Log("fuse-overlayfs missing — installing.")
		_ = runner.Run("apt-get", "install", "-y", "-qq", "fuse-overlayfs")
	}

	return runner.Run("machinectl", "shell", name+"@", "/bin/bash", "-c",
		"dockerd-rootless-setuptool.sh install")
}
