package cmd

import (
	"github.com/spf13/cobra"

	"lsm/internal/runner"
	"lsm/internal/state"
	"lsm/internal/ufw"
)

var firewallCmd = &cobra.Command{
	Use:   "firewall",
	Short: "Bootstrap UFW (idempotent): install, defaults, port 22 (only on first run), enable",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		runner.Section("UFW: idempotent bootstrap")

		if err := ufw.Install(); err != nil {
			return err
		}
		if err := ufw.SetDefaults(); err != nil {
			return err
		}

		// Port 22 bootstrap is only useful on first run, before the ssh
		// module has switched sshd to a non-default port. Re-running
		// firewall after ssh is installed must NOT reopen 22 — that would
		// undo the port-22 cleanup the user did after the SSH cutover.
		st, _ := state.Load(cfgFile)
		sshAlreadyInstalled := st != nil && st.IsInstalled("ssh")
		if sshAlreadyInstalled {
			runner.Log("ssh module already installed — skipping port 22 bootstrap.")
		} else if ufw.PortPermitted(22, "tcp") {
			runner.Log("Port 22 already permitted.")
		} else {
			if err := ufw.Allow(22, "tcp", "SSH bootstrap - remove after switching to new port"); err != nil {
				return err
			}
			runner.Log("Port 22 opened (bootstrap).")
		}
		if err := ufw.Enable(); err != nil {
			return err
		}

		// DOCKER-USER patch in after.rules. This is what makes `lsm port`
		// rules apply to docker-published ports — without it, `docker run
		// -p` bypasses UFW entirely. We install the block unconditionally
		// (cheap, idempotent, harmless on hosts without docker because the
		// DOCKER-USER chain only exists when dockerd is running).
		runner.Section("UFW: DOCKER-USER block (after.rules)")
		alreadyPresent := ufw.HasAfterRulesDockerBlock()
		if err := ufw.WriteAfterRulesDockerBlock(); err != nil {
			return err
		}
		if alreadyPresent {
			runner.Log("after.rules: DOCKER-USER block already present.")
		} else {
			runner.Log("after.rules: DOCKER-USER block written.")
		}
		if err := ufw.Reload(); err != nil {
			return err
		}

		markInstalled("firewall")
		return nil
	},
}

func init() { rootCmd.AddCommand(firewallCmd) }
