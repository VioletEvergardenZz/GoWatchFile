package sysinfo

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	defaultProcessLimit = 200
	defaultEnvLimit     = 20
	defaultCacheTTL     = 1 * time.Second
)

// Options 用于配置采集器的默认行为
type Options struct {
	CacheTTL     time.Duration
	ProcessLimit int
	EnvLimit     int
}

// SnapshotOptions 控制单次快照的内容
type SnapshotOptions struct {
	IncludeProcesses bool
	ProcessLimit     int
}

type cpuSample struct {
	total float64
	idle  float64
}

type diskSample struct {
	readBytes  uint64
	writeBytes uint64
}

type procSample struct {
	cpuTotal   float64
	readBytes  uint64
	writeBytes uint64
	hasIO      bool
}

type cacheKey struct {
	includeProcesses bool
	limit            int
}

// Collector 负责采集系统资源快照
type Collector struct {
	mu                  sync.Mutex
	cacheTTL            time.Duration
	defaultProcessLimit int
	envLimit            int

	lastSnapshot   SystemDashboard
	lastSnapshotAt time.Time
	lastKey        cacheKey

	lastCPU    cpuSample
	lastDisk   map[string]diskSample
	lastProc   map[int32]procSample
	lastSample time.Time
}

// NewCollector 创建系统信息采集器
func NewCollector(opts Options) *Collector {
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	processLimit := opts.ProcessLimit
	if processLimit <= 0 {
		processLimit = defaultProcessLimit
	}
	envLimit := opts.EnvLimit
	if envLimit <= 0 {
		envLimit = defaultEnvLimit
	}
	return &Collector{
		cacheTTL:            cacheTTL,
		defaultProcessLimit: processLimit,
		envLimit:            envLimit,
		lastDisk:            make(map[string]diskSample),
		lastProc:            make(map[int32]procSample),
	}
}

// Snapshot 返回系统资源面板快照
func (c *Collector) Snapshot(opts SnapshotOptions) (SystemDashboard, error) {
	now := time.Now()
	includeProcesses := opts.IncludeProcesses
	limit := opts.ProcessLimit
	if limit < 0 {
		limit = c.defaultProcessLimit
	}
	key := cacheKey{includeProcesses: includeProcesses, limit: limit}

	c.mu.Lock()
	if c.cacheTTL > 0 && now.Sub(c.lastSnapshotAt) < c.cacheTTL && c.lastKey == key {
		snapshot := c.lastSnapshot
		c.mu.Unlock()
		return snapshot, nil
	}
	prevCPU := c.lastCPU
	prevDisk := copyDiskSamples(c.lastDisk)
	prevProc := copyProcSamples(c.lastProc)
	prevSample := c.lastSample
	c.mu.Unlock()

	hostName, osName, kernel, uptime := collectHostInfo()
	loadLabel := collectLoadLabel()
	ip := firstIPv4()

	cpuInfoLabel := buildCPUInfoLabel()
	cpuUsage, cpuSample := collectCPUUsage(prevCPU)
	cpuTemp := collectCPUTemp()

	memGauge, memSub, memTrend := collectMemoryGauge()
	volumes, diskTotals := collectVolumes()
	diskReadRate, diskWriteRate, diskSample := collectDiskRates(prevDisk, prevSample, now)

	connStats, connBreakdown, portMap := collectConnections(includeProcesses)

	procCount, processes, procSamples, topProcess := collectProcesses(collectProcessOptions{
		include:    includeProcesses,
		limit:      limit,
		prevCPU:    prevCPU,
		currCPU:    cpuSample,
		prevSample: prevSample,
		prevProc:   prevProc,
		portMap:    portMap,
		envLimit:   c.envLimit,
	})

	cpuGauge := ResourceGauge{
		ID:         "cpu",
		Label:      "CPU 使用率",
		UsedPct:    cpuUsage,
		UsedLabel:  fmt.Sprintf("%.1f%%", cpuUsage),
		TotalLabel: cpuInfoLabel,
		SubLabel:   loadLabel,
		Trend:      formatCPUGaugeTrend(cpuUsage, cpuTemp),
		Tone:       usageTone(cpuUsage),
	}

	memGauge.Label = "内存占用"
	memGauge.ID = "memory"
	memGauge.SubLabel = memSub
	memGauge.Trend = memTrend
	memGauge.Tone = usageTone(memGauge.UsedPct)

	diskUsedPct := diskTotals.usedPct
	diskGauge := ResourceGauge{
		ID:         "disk",
		Label:      "磁盘占用",
		UsedPct:    diskUsedPct,
		UsedLabel:  diskTotals.usedLabel,
		TotalLabel: diskTotals.totalLabel,
		SubLabel:   fmt.Sprintf("读取 %s · 写入 %s", diskReadRate, diskWriteRate),
		Trend:      fmt.Sprintf("分区 %d 个", len(volumes)),
		Tone:       usageTone(diskUsedPct),
	}

	overview := Overview{
		Host:                 hostName,
		OS:                   osName,
		Kernel:               kernel,
		Uptime:               uptime,
		Load:                 loadLabel,
		IP:                   ip,
		LastUpdated:          now.Format("15:04:05"),
		Processes:            procCount,
		Connections:          connStats.total,
		ConnectionsBreakdown: connBreakdown,
		CPUTemp:              cpuTemp,
		TopProcess:           topProcess,
	}

	dashboard := SystemDashboard{
		SystemOverview:  overview,
		SystemGauges:    []ResourceGauge{cpuGauge, memGauge, diskGauge},
		SystemVolumes:   volumes,
		SystemProcesses: processes,
	}

	c.mu.Lock()
	c.lastSnapshot = dashboard
	c.lastSnapshotAt = now
	c.lastKey = key
	c.lastCPU = cpuSample
	c.lastDisk = diskSample
	c.lastProc = procSamples
	c.lastSample = now
	c.mu.Unlock()

	return dashboard, nil
}

func collectHostInfo() (string, string, string, string) {
	info, err := host.Info()
	if err != nil {
		name, _ := os.Hostname()
		return fallbackString(name, "--"),
			runtime.GOOS,
			"--",
			"--"
	}
	hostName := fallbackString(info.Hostname, "--")
	osName := strings.TrimSpace(strings.Join([]string{info.Platform, info.PlatformVersion}, " "))
	if osName == "" {
		osName = runtime.GOOS
	}
	kernel := fallbackString(info.KernelVersion, "--")
	uptime := formatDurationCN(time.Duration(info.Uptime) * time.Second)
	return hostName, osName, kernel, uptime
}

func collectLoadLabel() string {
	avg, err := load.Avg()
	if err != nil {
		return "--"
	}
	return fmt.Sprintf("%.2f / %.2f / %.2f", avg.Load1, avg.Load5, avg.Load15)
}

func buildCPUInfoLabel() string {
	cores := runtime.NumCPU()
	mhz := detectCPUMHz()
	if mhz <= 0 {
		infos, err := cpu.Info()
		if err == nil && len(infos) > 0 {
			if freq := sanitizeMHz(infos[0].Mhz); freq > 0 {
				mhz = freq
			}
			if mhz <= 0 {
				mhz = parseBrandMHz(infos[0].ModelName)
			}
		}
	}
	if mhz <= 0 {
		return fmt.Sprintf("%d 核", cores)
	}
	return fmt.Sprintf("%d 核 · %.1f GHz", cores, mhz/1000)
}

func sanitizeMHz(mhz float64) float64 {
	// 部分平台会返回极小值（如 24 MHz），直接视为未知
	if mhz < 100 {
		return 0
	}
	return mhz
}

func parseBrandMHz(brand string) float64 {
	if strings.TrimSpace(brand) == "" {
		return 0
	}
	re := regexp.MustCompile(`(?i)([0-9]+(?:\\.[0-9]+)?)\\s*ghz`)
	matches := re.FindStringSubmatch(brand)
	if len(matches) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	return val * 1000
}

// collectCPUUsage 基于两次采样的差值计算 CPU 使用率
func collectCPUUsage(prev cpuSample) (float64, cpuSample) {
	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return 0, cpuSample{}
	}
	curr := cpuSample{
		total: sumCPUTimes(times[0]),
		idle:  times[0].Idle + times[0].Iowait,
	}
	if prev.total <= 0 {
		// 第一次采样用短间隔获取更接近实时的 CPU 使用率
		percents, err := cpu.Percent(120*time.Millisecond, false)
		if err == nil && len(percents) > 0 {
			return clampPct(percents[0]), curr
		}
	}
	deltaTotal := curr.total - prev.total
	deltaIdle := curr.idle - prev.idle
	if deltaTotal > 0 {
		// 通过总量差值计算整体 CPU 使用率，避免每次阻塞采样
		used := (deltaTotal - deltaIdle) / deltaTotal * 100
		return clampPct(used), curr
	}
	return 0, curr
}

func collectCPUTemp() string {
	temps, err := host.SensorsTemperatures()
	if err != nil || len(temps) == 0 {
		return "--"
	}
	maxTemp := 0.0
	for _, sensor := range temps {
		if sensor.Temperature > maxTemp {
			maxTemp = sensor.Temperature
		}
	}
	if maxTemp <= 0 {
		return "--"
	}
	return fmt.Sprintf("%.0fC", maxTemp)
}

func formatCPUGaugeTrend(cpuUsage float64, cpuTemp string) string {
	idle := 100 - cpuUsage
	if cpuTemp == "--" {
		return fmt.Sprintf("空闲 %.1f%%", idle)
	}
	return fmt.Sprintf("空闲 %.1f%% · 温度 %s", idle, cpuTemp)
}

func collectMemoryGauge() (ResourceGauge, string, string) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return ResourceGauge{
			UsedPct:    0,
			UsedLabel:  "--",
			TotalLabel: "总计 --",
		}, "--", "--"
	}
	swap, _ := mem.SwapMemory()
	cache := vm.Cached
	if cache <= 0 && vm.Inactive > 0 {
		cache = vm.Inactive
	}
	if cache <= 0 && vm.Buffers > 0 {
		cache = vm.Buffers
	}
	usedLabel := formatBytes(float64(vm.Used))
	totalLabel := fmt.Sprintf("总计 %s", formatBytes(float64(vm.Total)))
	subLabel := fmt.Sprintf("缓存 %s · 交换 %s/%s", formatBytes(float64(cache)), formatBytes(float64(swap.Used)), formatBytes(float64(swap.Total)))
	trend := fmt.Sprintf("可用 %s", formatBytes(float64(vm.Available)))
	return ResourceGauge{
		UsedPct:    clampPct(vm.UsedPercent),
		UsedLabel:  usedLabel,
		TotalLabel: totalLabel,
	}, subLabel, trend
}

type diskTotals struct {
	usedPct    float64
	usedLabel  string
	totalLabel string
}

func collectVolumes() ([]Volume, diskTotals) {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return []Volume{}, diskTotals{usedPct: 0, usedLabel: "--", totalLabel: "总计 --"}
	}
	seenMount := make(map[string]struct{})
	seenDevice := make(map[string]struct{})
	volumes := make([]Volume, 0, len(partitions))
	var total uint64
	var used uint64
	for _, part := range partitions {
		if part.Mountpoint == "" || shouldSkipVolume(part) {
			continue
		}
		if _, ok := seenMount[part.Mountpoint]; ok {
			continue
		}
		seenMount[part.Mountpoint] = struct{}{}
		usage, err := disk.Usage(part.Mountpoint)
		if err != nil || usage.Total == 0 {
			continue
		}
		volumes = append(volumes, Volume{
			Mount:   part.Mountpoint,
			UsedPct: clampPct(usage.UsedPercent),
			Used:    formatBytes(float64(usage.Used)),
			Total:   formatBytes(float64(usage.Total)),
		})
		// 同一物理设备可能挂载多个 APFS 卷，避免累计总量时重复计算
		if _, ok := seenDevice[part.Device]; !ok {
			total += usage.Total
			used += usage.Used
			seenDevice[part.Device] = struct{}{}
		}
	}
	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Mount < volumes[j].Mount
	})
	usedPct := 0.0
	if total > 0 {
		usedPct = (float64(used) / float64(total)) * 100
	}
	return volumes, diskTotals{
		usedPct:    clampPct(usedPct),
		usedLabel:  formatBytes(float64(used)),
		totalLabel: fmt.Sprintf("总计 %s", formatBytes(float64(total))),
	}
}

func shouldSkipVolume(part disk.PartitionStat) bool {
	mount := strings.TrimSpace(part.Mountpoint)
	if mount == "" {
		return true
	}
	// 跳过设备与虚拟卷，避免伪分区撑高总量
	if mount == "/dev" || strings.HasPrefix(mount, "/dev/") {
		return true
	}
	// macOS 会暴露大量 APFS 卷（Preboot/VM 等），仅保留常规挂载
	if runtime.GOOS == "darwin" && strings.HasPrefix(mount, "/System/Volumes/") {
		return true
	}
	return false
}

// collectDiskRates 根据磁盘 IO 累计值计算读写速率
func collectDiskRates(prev map[string]diskSample, prevSample time.Time, now time.Time) (string, string, map[string]diskSample) {
	current, err := disk.IOCounters()
	if err != nil || len(current) == 0 {
		return "--", "--", map[string]diskSample{}
	}
	deltaRead := uint64(0)
	deltaWrite := uint64(0)
	next := make(map[string]diskSample, len(current))
	for name, stat := range current {
		next[name] = diskSample{readBytes: stat.ReadBytes, writeBytes: stat.WriteBytes}
		prevStat, ok := prev[name]
		if !ok {
			continue
		}
		if stat.ReadBytes >= prevStat.readBytes {
			deltaRead += stat.ReadBytes - prevStat.readBytes
		}
		if stat.WriteBytes >= prevStat.writeBytes {
			deltaWrite += stat.WriteBytes - prevStat.writeBytes
		}
	}
	interval := now.Sub(prevSample)
	if prevSample.IsZero() || interval <= 0 {
		return "--", "--", next
	}
	readRate := formatRate(deltaRead, interval)
	writeRate := formatRate(deltaWrite, interval)
	return readRate, writeRate, next
}

type connectionStats struct {
	total int
}

func collectConnections(includePorts bool) (connectionStats, string, map[int32][]string) {
	stats := connectionStats{}
	connBreakdown := "--"
	portMap := make(map[int32][]string)
	conns, err := gnet.Connections("inet")
	if err != nil || len(conns) == 0 {
		return stats, connBreakdown, portMap
	}

	counts := make(map[string]int)
	for _, conn := range conns {
		status := strings.ToUpper(conn.Status)
		counts[status]++
		if includePorts && status == "LISTEN" && conn.Pid > 0 {
			addr := formatAddr(conn.Laddr.IP, conn.Laddr.Port)
			if addr != "" {
				portMap[conn.Pid] = append(portMap[conn.Pid], addr)
			}
		}
	}

	stats.total = countNonListen(counts)
	if stats.total == 0 {
		stats.total = len(conns)
	}
	connBreakdown = formatConnectionBreakdown(counts)
	if includePorts {
		for pid, ports := range portMap {
			portMap[pid] = uniqueSorted(ports)
		}
	}
	return stats, connBreakdown, portMap
}

type collectProcessOptions struct {
	include    bool
	limit      int
	prevCPU    cpuSample
	currCPU    cpuSample
	prevSample time.Time
	prevProc   map[int32]procSample
	portMap    map[int32][]string
	envLimit   int
}

type procCandidate struct {
	proc      *process.Process
	pid       int32
	name      string
	command   string
	user      string
	status    string
	cpuPct    float64
	memPct    float64
	rss       uint64
	threads   int32
	startTime time.Time
	cwd       string
	exe       string
}

func collectProcesses(opts collectProcessOptions) (int, []Process, map[int32]procSample, string) {
	procs, err := process.Processes()
	if err != nil {
		return 0, []Process{}, map[int32]procSample{}, "--"
	}
	procCount := len(procs)
	if !opts.include {
		return procCount, []Process{}, map[int32]procSample{}, "--"
	}
	candidates := make([]procCandidate, 0, procCount)
	nextSamples := make(map[int32]procSample, len(procs))

	deltaTotal := opts.currCPU.total - opts.prevCPU.total
	numCPU := float64(runtime.NumCPU())
	if numCPU < 1 {
		numCPU = 1
	}
	for _, proc := range procs {
		pid := proc.Pid
		name, _ := proc.Name()
		if name == "" {
			name = fmt.Sprintf("pid-%d", pid)
		}
		statusRaw, _ := proc.Status()
		status := normalizeStatus(strings.Join(statusRaw, " "))
		cmdline, _ := proc.Cmdline()
		if cmdline == "" {
			cmdline = name
		}
		user, _ := proc.Username()
		if user == "" {
			user = "--"
		}
		createTime, _ := proc.CreateTime()
		startTime := time.Time{}
		if createTime > 0 {
			startTime = time.UnixMilli(createTime)
		}
		threads, _ := proc.NumThreads()
		cwd, _ := proc.Cwd()
		if cwd == "" {
			cwd = "--"
		}
		exe, _ := proc.Exe()
		if exe == "" {
			exe = "--"
		}

		// 某些进程（尤其是受限权限的系统进程）可能返回 nil 的 CPU 时间，需做防御性判空
		cpuTimes, _ := proc.Times()
		cpuTotal := 0.0
		if cpuTimes != nil {
			cpuTotal = cpuTimes.User + cpuTimes.System
		}
		nextSamples[pid] = procSample{cpuTotal: cpuTotal}

		cpuPct := 0.0
		prev, ok := opts.prevProc[pid]
		if ok && deltaTotal > 0 && cpuTotal >= prev.cpuTotal {
			// 以 CPU 总量差值估算进程占用比例，避免逐进程阻塞采样
			cpuPct = (cpuTotal - prev.cpuTotal) / deltaTotal * 100 * numCPU
			if cpuPct < 0 {
				cpuPct = 0
			}
		}

		// macOS 上很多进程状态标记为 sleeping，但若 CPU 明显占用则视为运行中，方便前端筛选
		if status != "running" && cpuPct >= 1.0 {
			status = "running"
		}

		memPct, _ := proc.MemoryPercent()
		memInfo, _ := proc.MemoryInfo()
		var rss uint64
		if memInfo != nil {
			rss = memInfo.RSS
		}

		candidates = append(candidates, procCandidate{
			proc:      proc,
			pid:       pid,
			name:      name,
			command:   cmdline,
			user:      user,
			status:    status,
			cpuPct:    cpuPct,
			memPct:    clampPct(float64(memPct)),
			rss:       rss,
			threads:   threads,
			startTime: startTime,
			cwd:       cwd,
			exe:       exe,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].cpuPct == candidates[j].cpuPct {
			return candidates[i].memPct > candidates[j].memPct
		}
		return candidates[i].cpuPct > candidates[j].cpuPct
	})

	if opts.limit > 0 && len(candidates) > opts.limit {
		candidates = candidates[:opts.limit]
	}

	processes := make([]Process, 0, len(candidates))
	interval := time.Duration(0)
	if !opts.prevSample.IsZero() {
		interval = time.Since(opts.prevSample)
	}
	topProcess := "--"
	for idx, candidate := range candidates {
		if idx == 0 {
			topProcess = candidate.name
		}
		ioRead, ioWrite, ioSample := collectProcessIO(candidate.proc, opts.prevProc[candidate.pid], interval)
		envs := collectProcessEnv(candidate.proc, opts.envLimit)
		ports := opts.portMap[candidate.pid]
		status := candidate.status
		if status != "running" && len(ports) > 0 {
			status = "running"
		}
		note := buildProcessNote(candidate.cpuPct, candidate.memPct)
		startLabel := "--"
		uptimeLabel := "--"
		if !candidate.startTime.IsZero() {
			startLabel = candidate.startTime.Format("15:04:05")
			uptimeLabel = formatDurationCN(time.Since(candidate.startTime))
		}

		// 记录 IO 的累计值，供下一次计算速率使用
		if ioSample.hasIO {
			prevSample := nextSamples[candidate.pid]
			prevSample.readBytes = ioSample.readBytes
			prevSample.writeBytes = ioSample.writeBytes
			prevSample.hasIO = true
			nextSamples[candidate.pid] = prevSample
		}

		processes = append(processes, Process{
			PID:     candidate.pid,
			Name:    candidate.name,
			Command: candidate.command,
			User:    candidate.user,
			Status:  status,
			CPU:     candidate.cpuPct,
			Mem:     candidate.memPct,
			RSS:     formatBytes(float64(candidate.rss)),
			Threads: candidate.threads,
			Start:   startLabel,
			Uptime:  uptimeLabel,
			Ports:   ports,
			IORead:  ioRead,
			IOWrite: ioWrite,
			NetIn:   "--",
			NetOut:  "--",
			Cwd:     candidate.cwd,
			Path:    candidate.exe,
			Env:     envs,
			Note:    note,
		})
	}
	return procCount, processes, nextSamples, topProcess
}

func collectProcessIO(proc *process.Process, prev procSample, interval time.Duration) (string, string, procSample) {
	ioCounters, err := proc.IOCounters()
	if err != nil || ioCounters == nil {
		return "--", "--", procSample{}
	}
	sample := procSample{
		readBytes:  ioCounters.ReadBytes,
		writeBytes: ioCounters.WriteBytes,
		hasIO:      true,
	}
	if interval <= 0 || !prev.hasIO {
		return "--", "--", sample
	}
	readDelta := uint64(0)
	writeDelta := uint64(0)
	if ioCounters.ReadBytes >= prev.readBytes {
		readDelta = ioCounters.ReadBytes - prev.readBytes
	}
	if ioCounters.WriteBytes >= prev.writeBytes {
		writeDelta = ioCounters.WriteBytes - prev.writeBytes
	}
	return formatRate(readDelta, interval), formatRate(writeDelta, interval), sample
}

func collectProcessEnv(proc *process.Process, limit int) []string {
	if limit <= 0 {
		return []string{}
	}
	env, err := proc.Environ()
	if err != nil || len(env) == 0 {
		return []string{}
	}
	if len(env) > limit {
		return env[:limit]
	}
	return env
}

func buildProcessNote(cpuPct, memPct float64) string {
	switch {
	case cpuPct >= 60 && memPct >= 20:
		return "CPU/内存偏高"
	case cpuPct >= 60:
		return "高 CPU 负载"
	case memPct >= 20:
		return "内存占用偏高"
	default:
		return ""
	}
}

func copyDiskSamples(src map[string]diskSample) map[string]diskSample {
	if len(src) == 0 {
		return map[string]diskSample{}
	}
	dst := make(map[string]diskSample, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyProcSamples(src map[int32]procSample) map[int32]procSample {
	if len(src) == 0 {
		return map[int32]procSample{}
	}
	dst := make(map[int32]procSample, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func sumCPUTimes(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
