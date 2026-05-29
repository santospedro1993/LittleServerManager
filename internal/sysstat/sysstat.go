// Package sysstat provides a tiny snapshot reader for /proc-derived host
// stats: CPU usage, memory, disk usage per filesystem, and per-NIC network
// throughput. It deliberately uses no external dependencies — everything
// comes from /proc and a couple of shell-outs (df).
//
// Future extension (tracked as a TODO): per-container equivalents
// reading from cgroup v2 + the docker socket.
package sysstat

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"erp24/internal/runner"
)

// CPUTimes holds the raw cumulative jiffies counters from /proc/stat (cpu line).
type CPUTimes struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
}

func (c CPUTimes) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}
func (c CPUTimes) IdleAll() uint64 { return c.Idle + c.IOWait }

// ReadCPU parses the aggregate `cpu` line from /proc/stat.
func ReadCPU() (CPUTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return CPUTimes{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		nums := make([]uint64, 8)
		for i := 0; i < 8 && i < len(fields); i++ {
			nums[i], _ = strconv.ParseUint(fields[i], 10, 64)
		}
		return CPUTimes{
			User: nums[0], Nice: nums[1], System: nums[2], Idle: nums[3],
			IOWait: nums[4], IRQ: nums[5], SoftIRQ: nums[6], Steal: nums[7],
		}, nil
	}
	return CPUTimes{}, fmt.Errorf("no `cpu` line in /proc/stat")
}

// CPUUsagePercent samples /proc/stat twice (sleep `sample` between) and
// returns the percentage of busy time. Sample 200ms–1s for snappy reads.
func CPUUsagePercent(sample time.Duration) (float64, error) {
	a, err := ReadCPU()
	if err != nil {
		return 0, err
	}
	time.Sleep(sample)
	b, err := ReadCPU()
	if err != nil {
		return 0, err
	}
	totalDelta := b.Total() - a.Total()
	idleDelta := b.IdleAll() - a.IdleAll()
	if totalDelta == 0 {
		return 0, nil
	}
	return float64(totalDelta-idleDelta) * 100.0 / float64(totalDelta), nil
}

// MemInfo holds a few /proc/meminfo fields, all in bytes.
type MemInfo struct {
	Total, Free, Available, Buffers, Cached, SwapTotal, SwapFree uint64
}

func (m MemInfo) Used() uint64     { return m.Total - m.Available }
func (m MemInfo) UsedSwap() uint64 { return m.SwapTotal - m.SwapFree }
func (m MemInfo) UsedPercent() float64 {
	if m.Total == 0 {
		return 0
	}
	return float64(m.Used()) * 100.0 / float64(m.Total)
}

func ReadMem() (MemInfo, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemInfo{}, err
	}
	defer f.Close()
	m := MemInfo{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		v *= 1024 // /proc/meminfo reports kB
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			m.Total = v
		case "MemFree":
			m.Free = v
		case "MemAvailable":
			m.Available = v
		case "Buffers":
			m.Buffers = v
		case "Cached":
			m.Cached = v
		case "SwapTotal":
			m.SwapTotal = v
		case "SwapFree":
			m.SwapFree = v
		}
	}
	return m, nil
}

// DiskUsage represents a single mounted filesystem, totals in bytes.
type DiskUsage struct {
	Filesystem        string
	Mount             string
	FSType            string
	Size, Used, Avail uint64
	UsedPercent       float64
}

// ReadDisks shells out to `df -PB1 -x tmpfs -x devtmpfs -x squashfs --output=...`.
// Falls back to a plain `df -B1` parse if --output isn't supported.
func ReadDisks() ([]DiskUsage, error) {
	out, err := runner.Capture("df", "-PB1",
		"-x", "tmpfs", "-x", "devtmpfs", "-x", "squashfs", "-x", "overlay",
	)
	if err != nil {
		return nil, err
	}
	var disks []DiskUsage
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		size, _ := strconv.ParseUint(fields[1], 10, 64)
		used, _ := strconv.ParseUint(fields[2], 10, 64)
		avail, _ := strconv.ParseUint(fields[3], 10, 64)
		pct := 0.0
		if size > 0 {
			pct = float64(used) * 100.0 / float64(size)
		}
		disks = append(disks, DiskUsage{
			Filesystem: fields[0],
			Mount:      fields[5],
			Size:       size, Used: used, Avail: avail,
			UsedPercent: pct,
		})
	}
	return disks, nil
}

// NetCounters maps interface name → (RX bytes, TX bytes) cumulative.
type NetCounters map[string][2]uint64

func ReadNet() (NetCounters, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := NetCounters{}
	sc := bufio.NewScanner(f)
	// First two lines are headers.
	sc.Scan()
	sc.Scan()
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		out[name] = [2]uint64{rx, tx}
	}
	return out, nil
}

// NetRate samples /proc/net/dev twice, returns bytes/second per interface
// (rx, tx). Loopback `lo` is excluded.
func NetRate(sample time.Duration) (map[string][2]float64, error) {
	a, err := ReadNet()
	if err != nil {
		return nil, err
	}
	time.Sleep(sample)
	b, err := ReadNet()
	if err != nil {
		return nil, err
	}
	secs := sample.Seconds()
	if secs == 0 {
		secs = 1
	}
	out := map[string][2]float64{}
	for name, after := range b {
		if name == "lo" {
			continue
		}
		before, ok := a[name]
		if !ok {
			continue
		}
		dRx := float64(after[0]-before[0]) / secs
		dTx := float64(after[1]-before[1]) / secs
		out[name] = [2]float64{dRx, dTx}
	}
	return out, nil
}

// CPUCount returns the number of online logical CPUs.
func CPUCount() int { return runtime.NumCPU() }

// CPUFreq holds clock-frequency info for the host. All values are in MHz.
// AvgCur is the mean current frequency across online cores; Max is the
// per-core hardware maximum (cpuinfo_max_freq); MaxAggregate is Max * cores
// — the upper bound of compute the box can deliver right now.
type CPUFreq struct {
	AvgCur       float64
	Max          float64
	MaxAggregate float64
	Source       string // "cpufreq" or "cpuinfo"
}

// ReadCPUFreq probes /sys/devices/system/cpu/*/cpufreq for live frequency
// values. Falls back to parsing /proc/cpuinfo "cpu MHz" lines when cpufreq
// isn't exposed (some virtualized environments hide it). Returns a zero
// CPUFreq with empty Source if neither source produced useful data.
func ReadCPUFreq() CPUFreq {
	out := CPUFreq{}

	// Preferred path: cpufreq sysfs.
	curs, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	if len(curs) > 0 {
		var sum float64
		var n int
		for _, p := range curs {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			if err != nil {
				continue
			}
			sum += v / 1000.0 // kHz → MHz
			n++
		}
		if n > 0 {
			out.AvgCur = sum / float64(n)
			out.Source = "cpufreq"
		}
		// Per-core max: same value on every core in the typical case.
		// Read cpu0's value as canonical.
		if b, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq"); err == nil {
			if v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
				out.Max = v / 1000.0
			}
		}
	}

	// Fallback: /proc/cpuinfo "cpu MHz".
	if out.Source == "" {
		f, err := os.Open("/proc/cpuinfo")
		if err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			var sum float64
			var n int
			for sc.Scan() {
				line := sc.Text()
				if !strings.HasPrefix(line, "cpu MHz") {
					continue
				}
				colon := strings.Index(line, ":")
				if colon < 0 {
					continue
				}
				v, err := strconv.ParseFloat(strings.TrimSpace(line[colon+1:]), 64)
				if err != nil {
					continue
				}
				sum += v
				n++
			}
			if n > 0 {
				out.AvgCur = sum / float64(n)
				out.Source = "cpuinfo"
			}
		}
	}

	if out.Max == 0 && out.AvgCur > 0 {
		// Best-effort: assume current sample IS the max if we couldn't
		// read cpuinfo_max_freq. Better than printing 0.
		out.Max = out.AvgCur
	}
	out.MaxAggregate = out.Max * float64(CPUCount())
	return out
}

// FormatBytes returns a short human-readable size (e.g. "12.3 GB").
func FormatBytes(n uint64) string {
	const k = 1024
	if n < k {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(k), 0
	for v := n / k; v >= k; v /= k {
		div *= k
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// FormatRate prints bytes/second with sensible units.
func FormatRate(bps float64) string {
	const k = 1024.0
	switch {
	case bps < k:
		return fmt.Sprintf("%.0f B/s", bps)
	case bps < k*k:
		return fmt.Sprintf("%.1f KiB/s", bps/k)
	case bps < k*k*k:
		return fmt.Sprintf("%.1f MiB/s", bps/(k*k))
	default:
		return fmt.Sprintf("%.1f GiB/s", bps/(k*k*k))
	}
}
