package cmd

import (
	"fmt"
	"os"
	"os/exec"
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
	snap := gatherStatus()
	renderStatus(snap)
	return nil
}

func statusLiveLoop() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	// Put the tty into non-canonical mode WITHOUT echo, with a 2s timeout
	// (time=20 deciseconds, min=0 = non-blocking poll). Each os.Stdin.Read
	// returns either when a key is pressed or after 2s — that's our refresh
	// cadence. No goroutine, no leftover blocked reader after exit.
	hadTTY := setRawTTYPoll(true)
	defer setRawTTYPoll(false)

	snap := gatherStatus()
	clearScreen()
	renderStatus(snap)

	buf := make([]byte, 1)
	for {
		select {
		case <-stop:
			fmt.Println()
			return nil
		default:
		}

		if hadTTY {
			n, _ := os.Stdin.Read(buf)
			if n > 0 {
				k := buf[0]
				if k == 'q' || k == 'Q' || k == 'x' || k == 'X' || k == 0x1b || k == '\r' || k == '\n' {
					fmt.Println()
					return nil
				}
				// any other key: redraw immediately (no wait)
			}
		} else {
			// No tty: fall back to a blocking 2s sleep + signal-only exit.
			time.Sleep(2 * time.Second)
		}

		snap := gatherStatus()
		clearScreen()
		renderStatus(snap)
	}
}

// setRawTTYPoll configures /dev/tty for single-char polling reads with a 2s
// timeout, and disables echo so keystrokes don't pollute the rendered frame.
// Restore-mode (enable=false) returns to canonical line mode + echo.
//
// We shell out to stty rather than pulling in golang.org/x/term — fewer deps.
// MIN=0 + TIME=20 makes Stdin.Read return after at most 2s even if nothing
// is typed; that's how the refresh ticker is implemented in this loop.
func setRawTTYPoll(enable bool) bool {
	args := []string{"-F", "/dev/tty"}
	if enable {
		args = append(args, "-icanon", "-echo", "min", "0", "time", "20")
	} else {
		args = append(args, "icanon", "echo")
	}
	return exec.Command("stty", args...).Run() == nil
}

// clearScreen uses the ANSI sequence so we don't depend on `clear`.
func clearScreen() { fmt.Print("\033[H\033[2J") }

// statusSnap holds one frame of host metrics. Captured outside renderStatus
// so we can sample first (which sleeps ~1s for CPU + net deltas) and only
// then clear the screen — eliminates the visible black gap in --live mode.
type statusSnap struct {
	host    string
	now     string
	cpuPct  float64
	cores   int
	freq    sysstat.CPUFreq
	mem     sysstat.MemInfo
	disks   []sysstat.DiskUsage
	netRate map[string][2]float64
}

func gatherStatus() statusSnap {
	host, _ := os.Hostname()
	cpuPct, _ := sysstat.CPUUsagePercent(200 * time.Millisecond)
	mem, _ := sysstat.ReadMem()
	disks, _ := sysstat.ReadDisks()
	netRate, _ := sysstat.NetRate(800 * time.Millisecond)
	return statusSnap{
		host:    host,
		now:     time.Now().Format("2006-01-02 15:04:05"),
		cpuPct:  cpuPct,
		cores:   sysstat.CPUCount(),
		freq:    sysstat.ReadCPUFreq(),
		mem:     mem,
		disks:   disks,
		netRate: netRate,
	}
}

func renderStatus(s statusSnap) {
	cpuPct, mem, disks, netRate := s.cpuPct, s.mem, s.disks, s.netRate

	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  Host status — %s — %-20s        ║\n", s.host, s.now)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	// CPU — capacity (static) on top, live status below.
	fmt.Println()
	if s.freq.Source != "" {
		fmt.Printf("CPU      %d logical cores · per-core max %s · aggregate max %s  (%s)\n",
			s.cores,
			formatMHz(s.freq.Max),
			formatMHz(s.freq.MaxAggregate),
			s.freq.Source)
		fmt.Printf("         load %5.1f%%  ·  avg clock %s\n", cpuPct, formatMHz(s.freq.AvgCur))
	} else {
		fmt.Printf("CPU      %d logical cores\n", s.cores)
		fmt.Printf("         load %5.1f%%\n", cpuPct)
	}
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
		fmt.Println("(refreshing every 2s — press q / x / Esc / Enter or Ctrl+C to exit)")
	}
}

// formatMHz prints a frequency in MHz or GHz depending on magnitude.
// Aggregated values across many cores easily exceed 10 GHz so the GHz
// branch keeps the output compact.
func formatMHz(mhz float64) string {
	if mhz >= 1000 {
		return fmt.Sprintf("%.2f GHz", mhz/1000)
	}
	return fmt.Sprintf("%.0f MHz", mhz)
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
