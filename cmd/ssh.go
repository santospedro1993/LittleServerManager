package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"erp24/internal/config"
	"erp24/internal/prompt"
	"erp24/internal/runner"
	sshmod "erp24/internal/ssh"
	"erp24/internal/state"
	"erp24/internal/ufw"
)

var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "SSH hardening: create user, change port, disable root, open UFW",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		if !ufw.Installed() {
			return fmt.Errorf("UFW não instalado. Corre primeiro: erp24 firewall")
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
		if err := sshmod.GrantPasswordlessERP24(cfg.SSH.User); err != nil {
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
			if st.AddPort(state.ManagedPort{Port: cfg.SSH.Port, Proto: "tcp", Label: "SSH", Kind: state.KindHost}) {
				if err := st.Save(); err != nil {
					return err
				}
			}

			// Cutover: fechar a regra bootstrap da porta 22. Só aqui, dentro do
			// branch que abriu a nova porta — garante que a 2210 está permitida
			// antes de remover a 22 (sem isto, auto_open_ports=false fecharia a
			// 22 sem nova porta aberta → lockout). sshd já foi recarregado por
			// Harden (escuta só na nova porta); a sessão atual é uma ligação
			// estabelecida que sobrevive ao delete da regra UFW. Logins novos
			// passam a usar a nova porta.
			if cfg.SSH.Port != 22 && ufw.PortPermitted(cfg.SSH.Port, "tcp") && ufw.PortPermitted(22, "tcp") {
				if confirmClose22(cfg.SSH.Port) {
					if err := ufw.DeleteAllow(22, "tcp"); err != nil {
						return err
					}
					runner.Log("UFW: porta 22 (bootstrap) fechada — SSH agora só na %d.", cfg.SSH.Port)
				} else {
					runner.Log("Porta 22 mantida. Fecha à mão depois de testar: ufw delete allow 22/tcp")
				}
			}
		}

		runner.Log("Para restringir por IP: erp24 add-ip <IP>")
		markInstalled("ssh")
		return nil
	},
}

// confirmClose22 gates closing the port-22 bootstrap rule. Returns true under
// --yes. The current session survives (established connection), but a misconfig
// on the new port would block future logins, so we confirm when interactive.
func confirmClose22(newPort int) bool {
	if yes {
		return true
	}
	fmt.Println()
	fmt.Printf("sshd já escuta na porta %d. Posso fechar a porta 22 (bootstrap)?\n", newPort)
	fmt.Println("  A tua sessão atual sobrevive. Logins novos passam a usar a nova porta.")
	fmt.Printf("  ⚠ Confirma que consegues ligar na porta %d antes de fechar.\n", newPort)
	return prompt.Confirm("Fechar porta 22 agora?", true)
}

func init() { rootCmd.AddCommand(sshCmd) }
