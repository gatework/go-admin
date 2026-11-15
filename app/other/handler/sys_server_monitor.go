package handler

import (
	"fmt"
	"runtime"
	"strconv"
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
	// B bounds b processing.
	B = 1
	// KB describes the kb used by this module.
	KB = 1024 * B
	MB = 1024 * KB
	GB = 1024 * MB
)

var excludeNetInterfaces = []string{
	"lo", "tun", "docker", "veth", "br-", "vmbr", "vnet", "kube",
}

// ServerMonitor carries server monitor state for this module.
type ServerMonitor struct {
	api.API
}

// GetHourDiffer 获取相差时间
func GetHourDiffer(startTime, endTime string) int64 {
	t1, err1 := time.ParseInLocation("2006-01-02 15:04:05", startTime, time.Local)
	t2, err2 := time.ParseInLocation("2006-01-02 15:04:05", endTime, time.Local)
	if err1 != nil || err2 != nil || !t1.Before(t2) {
		return 0
	}
	return (t2.Unix() - t1.Unix()) / 3600
}

// ServerInfo 获取系统信息
func (e ServerMonitor) ServerInfo(c *gin.Context) {
	e.Context = c

	osInfo := getOSInfo()
	memInfo := getMemoryInfo()
	swapInfo := getSwapInfo()
	cpuInfo := getCPUInfo()
	diskInfo := getDiskInfo()
	netInfo := getNetworkInfo()

	bootTime, _ := host.BootTime()
	cachedBootTime := time.Unix(int64(bootTime), 0)

	e.Custom(gin.H{
		"code":     200,
		"os":       osInfo,
		"mem":      memInfo,
		"cpu":      cpuInfo,
		"disk":     diskInfo,
		"net":      netInfo,
		"swap":     swapInfo,
		"location": "Aliyun",
		"bootTime": GetHourDiffer(cachedBootTime.Format("2006-01-02 15:04:05"), time.Now().Format("2006-01-02 15:04:05")),
	})
}

func getOSInfo() map[string]any {
	sysInfo, _ := host.Info()
	return map[string]any{
		"goOs":         runtime.GOOS,
		"arch":         runtime.GOARCH,
		"mem":          runtime.MemProfileRate,
		"compiler":     runtime.Compiler,
		"version":      runtime.Version(),
		"numGoroutine": runtime.NumGoroutine(),
		"ip":           pkg.GetLocalHost(),
		"projectDir":   pkg.GetCurrentPath(),
		"hostName":     sysInfo.Hostname,
		"time":         time.Now().Format("2006-01-02 15:04:05"),
	}
}

func getMemoryInfo() map[string]any {
	memory, _ := mem.VirtualMemory()
	return map[string]any{
		"used":    memory.Used / MB,
		"total":   memory.Total / MB,
		"percent": pkg.Round(memory.UsedPercent, 2),
	}
}

func getSwapInfo() map[string]any {
	memory, _ := mem.VirtualMemory()
	return map[string]any{
		"used":  memory.SwapTotal - memory.SwapFree,
		"total": memory.SwapTotal,
	}
}

func getCPUInfo() map[string]any {
	cpuInfo, _ := cpu.Info()
	percent, _ := cpu.Percent(0, false)
	cpuNum, _ := cpu.Counts(false)
	return map[string]any{
		"cpuInfo": cpuInfo,
		"percent": pkg.Round(percent[0], 2),
		"cpuNum":  cpuNum,
	}
}

func getDiskInfo() map[string]any {
	var diskTotal, diskUsed, diskUsedPercent float64

	d, _ := disk.Usage("/")
	diskTotal = float64(d.Total / GB)
	diskUsed = float64(d.Used / GB)
	diskUsedPercent, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", d.UsedPercent), 64)

	return map[string]any{
		"total":   diskTotal,
		"used":    diskUsed,
		"percent": diskUsedPercent,
	}
}

func getNetworkInfo() map[string]any {
	netInSpeed, netOutSpeed := trackNetworkSpeed()
	return map[string]any{
		"in":  pkg.Round(float64(netInSpeed/KB), 2),
		"out": pkg.Round(float64(netOutSpeed/KB), 2),
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
