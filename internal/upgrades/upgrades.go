package upgrades

import (
	"os"

	"lsm/internal/runner"
)

const periodicConf = "/etc/apt/apt.conf.d/20auto-upgrades"

func Installed() bool {
	_, err := os.Stat("/etc/apt/apt.conf.d/50unattended-upgrades")
	return err == nil
}

func Install() error {
	runner.Log("Garantir unattended-upgrades + apt-listchanges instalados...")
	if err := runner.Run("apt-get", "update", "-qq"); err != nil {
		return err
	}
	return runner.Run("apt-get", "install", "-y", "-qq",
		"unattended-upgrades", "apt-listchanges")
}

// EnablePeriodic writes /etc/apt/apt.conf.d/20auto-upgrades enabling
// daily update + unattended-upgrade + autoclean every 7 days.
func EnablePeriodic() error {
	body := `// Managed by lsm.
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
`
	if err := os.WriteFile(periodicConf, []byte(body), 0644); err != nil {
		return err
	}
	return runner.Run("systemctl", "enable", "--now", "unattended-upgrades")
}

func DryRunPreview() (string, error) {
	return runner.Capture("unattended-upgrade", "--dry-run", "-d")
}
