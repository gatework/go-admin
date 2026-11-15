package handler

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/net"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/pkg"
)

// Byte units used by the monitor response.
const (
	kib = 1024
	mib = 1024 * kib
	gib = 1024 * mib
)

var excludeNetInterfaces = []string{
	"lo", "tun", "docker", "veth", "br-", "vmbr", "vnet", "kube",
}

// ServerMonitor carries server monitor state for this module.
type ServerMonitor struct {
	api.API
}

// ServerInfo reports the host's resource use. Each probe that fails leaves its
// fields at zero rather than failing the whole answer; the failures are logged
// once. Probes used to ignore their errors and then dereference the result,
// which panicked on hosts (and containers) where one of them is unavailable.
func (e ServerMonitor) ServerInfo(c *gin.Context) {
	e.Context = c
	var failed []error
	report := func(err error) {
		if err != nil {
			failed = append(failed, err)
		}
	}
	var uptime int64
	if boot, err := host.BootTime(); err == nil {
		uptime = int64(time.Since(time.Unix(int64(boot), 0)).Hours())
	} else {
		report(fmt.Errorf("boot time: %w", err))
	}
	payload := gin.H{
		"code":     200,
		"os":       osInfo(report),
		"mem":      memoryInfo(report),
		"cpu":      cpuInfo(report),
		"disk":     diskInfo(report),
		"net":      getNetworkInfo(),
		"swap":     swapInfo(report),
		"bootTime": uptime,
	}
	if len(failed) > 0 {
		api.GetRequestLogger(c).Warnf("server monitor: %v", errors.Join(failed...))
	}
	e.Custom(payload)
}

func osInfo(report func(error)) map[string]any {
	info := map[string]any{
		"goOs":         runtime.GOOS,
		"arch":         runtime.GOARCH,
		"mem":          runtime.MemProfileRate,
		"compiler":     runtime.Compiler,
		"version":      runtime.Version(),
		"numGoroutine": runtime.NumGoroutine(),
		"ip":           pkg.GetLocalHost(),
		"projectDir":   pkg.GetCurrentPath(),
		"hostName":     "",
		"time":         time.Now().Format(time.DateTime),
	}
	if sysInfo, err := host.Info(); err == nil && sysInfo != nil {
		info["hostName"] = sysInfo.Hostname
	} else {
		report(fmt.Errorf("host: %w", err))
	}
	return info
}

func memoryInfo(report func(error)) map[string]any {
	memory, err := mem.VirtualMemory()
	if err != nil || memory == nil {
		report(fmt.Errorf("memory: %w", err))
		return map[string]any{"used": 0, "total": 0, "percent": 0}
	}
	return map[string]any{
		"used":    memory.Used / mib,
		"total":   memory.Total / mib,
		"percent": pkg.Round(memory.UsedPercent, 2),
	}
}

// swapInfo is in bytes; the UI scales it.
func swapInfo(report func(error)) map[string]any {
	swap, err := mem.SwapMemory()
	if err != nil || swap == nil {
		report(fmt.Errorf("swap: %w", err))
		return map[string]any{"used": 0, "total": 0}
	}
	return map[string]any{"used": swap.Used, "total": swap.Total}
}

func cpuInfo(report func(error)) map[string]any {
	info, err := cpu.Info()
	report(wrapProbe("cpu info", err))
	var usage float64
	percent, err := cpu.Percent(0, false)
	if err == nil && len(percent) > 0 {
		usage = pkg.Round(percent[0], 2)
	} else {
		report(fmt.Errorf("cpu usage: %w", err))
	}
	cores, err := cpu.Counts(false)
	report(wrapProbe("cpu count", err))
	return map[string]any{"cpuInfo": info, "percent": usage, "cpuNum": cores}
}

func diskInfo(report func(error)) map[string]any {
	usage, err := disk.Usage("/")
	if err != nil || usage == nil {
		report(fmt.Errorf("disk: %w", err))
		return map[string]any{"total": 0.0, "used": 0.0, "percent": 0.0}
	}
	return map[string]any{
		"total":   float64(usage.Total / gib),
		"used":    float64(usage.Used / gib),
		"percent": pkg.Round(usage.UsedPercent, 2),
	}
}

func wrapProbe(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", name, err)
}

func getNetworkInfo() map[string]any {
	netInSpeed, netOutSpeed := trackNetworkSpeed()
	return map[string]any{
		"in":  pkg.Round(float64(netInSpeed/kib), 2),
		"out": pkg.Round(float64(netOutSpeed/kib), 2),
	}
}

// networkSample serializes the cumulative counter baseline shared by monitor requests.
var networkSample struct {
	sync.Mutex
	at             time.Time
	received, sent uint64
}

func trackNetworkSpeed() (uint64, uint64) {
	counters, err := net.IOCounters(true)
	if err != nil {
		return 0, 0
	}
	var received, sent uint64
	for _, counter := range counters {
		if isListContainsStr(excludeNetInterfaces, counter.Name) {
			continue
		}
		received += counter.BytesRecv
		sent += counter.BytesSent
	}
	networkSample.Lock()
	defer networkSample.Unlock()
	now := time.Now()
	elapsed := now.Sub(networkSample.at).Seconds()
	var in, out uint64
	if !networkSample.at.IsZero() && elapsed > 0 {
		if received >= networkSample.received {
			in = uint64(float64(received-networkSample.received) / elapsed)
		}
		if sent >= networkSample.sent {
			out = uint64(float64(sent-networkSample.sent) / elapsed)
		}
	}
	networkSample.at, networkSample.received, networkSample.sent = now, received, sent
	return in, out
}

func isListContainsStr(list []string, str string) bool {
	for _, item := range list {
		if strings.Contains(str, item) {
			return true
		}
	}
	return false
}
