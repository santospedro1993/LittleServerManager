package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/prompt"
	"lsm/internal/runner"
	sshmod "lsm/internal/ssh"
	"lsm/internal/state"
	"lsm/internal/ufw"
)

var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "SSH hardening: create user, change port, disable root, open UFW",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		if !ufw.Installed() {
			return fmt.Errorf("UFW não instalado. Corre primeiro: lsm firewall")
		}
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		st, err := state.Load(cfgFile)
		if err != nil {
			return err
		}

		runner.Section(fmt.Sprintf("SSH: garantir user '%s' + hardening", cfg.SSH.User))

		var password string
		if !sshmod.UserExists(cfg.SSH.User) {
			password = prompt.AskPassword(fmt.Sprintf("Password p/ novo user '%s'", cfg.SSH.User))
		}
		if err := sshmod.CreateUser(cfg.SSH.User, password); err != nil {
			return err
		}
		if err := sshmod.Harden(cfg.SSH.Port); err != nil {
			return err
		}

		runner.Section(fmt.Sprintf("SSH: abrir porta %d em UFW", cfg.SSH.Port))
		if shouldOpen(fmt.Sprintf("porta SSH %d/tcp", cfg.SSH.Port), cfg.Network.AutoOpenPorts) {
			if err := sshmod.OpenFirewall(cfg.SSH.Port); err != nil {
				return err
			}
			if st.AddPort(state.ManagedPort{Port: cfg.SSH.Port, Proto: "tcp", Label: "SSH"}) {
				if err := st.Save(); err != nil {
					return err
				}
			}
		}

		runner.Log("Próximo passo: testar SSH na porta %d, depois  ufw delete allow 22/tcp", cfg.SSH.Port)
		runner.Log("Para restringir por IP: lsm add-ip <IP>")
		markInstalled("ssh")
		return nil
	},
}

func init() { rootCmd.AddCommand(sshCmd) }
