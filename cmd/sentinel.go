package cmd

import (
	"github.com/spf13/cobra"

	"erp24/internal/config"
	"erp24/internal/sentinel"
)

var sentinelCmd = &cobra.Command{
	Use:   "sentinel",
	Short: "Install + enrol the Sentinel monitoring agent (pull-only, no ports opened)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		// Central URL comes from config (intent). Empty is fine — the module
		// prompts with a default. The install key is asked at runtime (secret).
		central := ""
		if config.Exists(cfgFile) {
			if c, err := config.Load(cfgFile); err == nil {
				central = c.Sentinel.CentralURL
			}
		}
		if err := sentinel.Run(central, appVersion); err != nil {
			return err
		}
		markInstalled("sentinel")
		return nil
	},
}

func init() { rootCmd.AddCommand(sentinelCmd) }
