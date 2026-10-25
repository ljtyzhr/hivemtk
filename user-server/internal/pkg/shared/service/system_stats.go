package service

import (
	"os"
	"runtime"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/contract"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
)

// SystemStatsService 系统统计服务
type SystemStatsService struct{}

// NewSystemStatsService 创建系统统计服务
func NewSystemStatsService() *SystemStatsService {
	return &SystemStatsService{}
}

// SystemInfo 系统信息
type SystemInfo struct {
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	DiskUsage    float64 `json:"disk_usage"`
	Uptime       int64   `json:"uptime"`
	ServerTime   string  `json:"server_time"`
	Hostname     string  `json:"hostname"`
	GoVersion    string  `json:"go_version"`
	NumCPU       int     `json:"num_cpu"`
	NumGoroutine int     `json:"num_goroutine"`
	AllocMemory  uint64  `json:"alloc_memory"`
	SysMemory    uint64  `json:"sys_memory"`
	// PlatformEnabled 平台集成是否启用（PLATFORM_ENABLED）。/api/system/info 是 public 端点，
	// 前端启动即可拿到，用来隐藏那些"点了必然 403"的上架入口。
	PlatformEnabled bool `json:"platform_enabled"`

	// PlatformContract 平台最近一次以 **426** 拒绝本机的契约版本状态。
	//
	// 为什么必须出现在响应里而不只写日志：失败若只进日志，运维无从得知"平台在要求
	// 本机升级"。用指针 + `omitempty`：从未被拒过时整个字段不出现，off 模式（无平台）
	// 的响应体与改造前逐字节等价——这是硬约束。只进管理员视图：publicSystemInfo
	// 是白名单裁剪，不列此字段则不泄露平台契约版本。
	PlatformContract *contract.Status `json:"platform_contract,omitempty"`
}

// GetSystemInfo 获取系统信息
func (s *SystemStatsService) GetSystemInfo() (*SystemInfo, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	cpuPercent, err := cpu.Percent(0, false)
	cpuUsage := 0.0
	if err == nil && len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	diskUsage, err := disk.Usage("/")
	diskPct := 0.0
	if err == nil {
		diskPct = diskUsage.UsedPercent
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	uptime, err := host.Uptime()
	if err != nil {
		uptime = 0
	}

	memPct := 0.0
	if m.Sys > 0 {
		memPct = float64(m.Alloc) / float64(m.Sys) * 100
	}

	info := &SystemInfo{
		CPUUsage:     cpuUsage,
		MemoryUsage:  memPct,
		DiskUsage:    diskPct,
		Uptime:       int64(uptime),
		ServerTime:   time.Now().Format("2006-01-02 15:04:05"),
		Hostname:     hostname,
		GoVersion:    runtime.Version(),
		NumCPU:       runtime.NumCPU(),
		NumGoroutine: runtime.NumGoroutine(),
		AllocMemory:  m.Alloc,
		SysMemory:    m.Sys,

		PlatformEnabled: config.PlatformEnabled(),
	}
	// 只有**正在被拒**时才填：填空结构会让 omitempty 失效（off 模式逐字节等价被破坏）。
	if st, ok := contract.UpgradeRequired(); ok {
		info.PlatformContract = &st
	}
	return info, nil
}
