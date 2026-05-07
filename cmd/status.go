package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"lsm/internal/runner"
	"lsm/internal/sysstat"
)

// statusLive controls whether `lsm status` refreshes continuously or prints once.
var statusLive bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Live host status: CPU, RAM, disk, network",
	Long: `Show a snapshot of host load: CPU usage, memory, disk usage per
filesystem, and per-NIC network throughput. Pass --live to refresh
every 2 seconds (Ctrl+C to exit).

Operator-class command: dev24 can run it via 'sudo lsm status'.

TODO (post-MVP): per-container view of the same metrics for the
rootless docker user (cgroup v2 + docker stats).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		if statusLive {
			return statusLiveLoop()
		}
		return statusOnce()
	},
}

func init() {
	statusCmd.Flags().BoolVarP(&statusLive, "live", "l", false, "refresh every 2s (Ctrl+C to exit)")
	rootCmd.AddCommand(statusCmd)
}

func statusOnce() error {
	printStatus()
	return nil
}

func statusLiveLoop() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	t := time.NewTicker(2 * time.Second)
	defer t.Stop()

	clearScreen()
	printStatus()
	for {
		select {
		case <-stop:
			fmt.Println()
			return nil
		case <-t.C:
			clearScreen()
			printStatus()
		}
	}
}

// clearScreen uses the ANSI sequence so we don't depend on `clear`.
func clearScreen() { fmt.Print("\033[H\033[2J") }

func printStatus() {
	host, _ := os.Hostname()
	now := time.Now().Format("2006-01-02 15:04:05")

	cpuPct, _ := sysstat.CPUUsagePercent(200 * time.Millisecond)
	mem, _ := sysstat.ReadMem()
	disks, _ := sysstat.ReadDisks()
	netRate, _ := sysstat.NetRate(800 * time.Millisecond)

	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  Host status — %s — %-20s        ║\n", host, now)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	// CPU
	fmt.Println()
	fmt.Printf("CPU      %5.1f%%  on %d logical cores\n", cpuPct, sysstat.CPUCount())
	fmt.Printf("         %s\n", bar(cpuPct, 50))

	// Memory
	fmt.Println()
	fmt.Printf("Memory   %5.1f%%  used  (%s / %s)\n",
		mem.UsedPercent(), sysstat.FormatBytes(mem.Used()), sysstat.FormatBytes(mem.Total))
	fmt.Printf("         %s\n", bar(mem.UsedPercent(), 50))
	if mem.SwapTotal > 0 {
		swapPct := 0.0
		if mem.SwapTotal > 0 {
			swapPct = float64(mem.UsedSwap()) * 100.0 / float64(mem.SwapTotal)
		}
		fmt.Printf("Swap     %5.1f%%  used  (%s / %s)\n",
			swapPct, sysstat.FormatBytes(mem.UsedSwap()), sysstat.FormatBytes(mem.SwapTotal))
	} else {
		fmt.Println("Swap     (none)")
	}

	// Disks
	fmt.Println()
	fmt.Println("Disk")
	if len(disks) == 0 {
		fmt.Println("  (no filesystems reported)")
	} else {
		for _, d := range disks {
			fmt.Printf("  %-30s %5.1f%%  (%s / %s)\n",
				d.Mount, d.UsedPercent,
				sysstat.FormatBytes(d.Used), sysstat.FormatBytes(d.Size))
		}
	}

	// Network
	fmt.Println()
	fmt.Println("Network (per second, 0.8s sample)")
	if len(netRate) == 0 {
		fmt.Println("  (no non-loopback interfaces)")
	} else {
		for name, rates := range netRate {
			fmt.Printf("  %-12s  ↓ %-12s  ↑ %s\n",
				name, sysstat.FormatRate(rates[0]), sysstat.FormatRate(rates[1]))
		}
	}

	if statusLive {
		fmt.Println()
		fmt.Println("(refreshing every 2s — Ctrl+C to exit)")
	}
}

// bar renders a simple ASCII progress bar for percent values.
func bar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	out := make([]rune, 0, width+2)
	out = append(out, '[')
	for i := 0; i < width; i++ {
		if i < filled {
			out = append(out, '█')
		} else {
			out = append(out, ' ')
		}
	}
	out = append(out, ']')
	return string(out)
}
