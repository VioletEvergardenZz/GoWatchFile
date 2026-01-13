package sysinfo

// Overview 表示系统概览信息
type Overview struct {
	Host                 string `json:"host"`
	OS                   string `json:"os"`
	Kernel               string `json:"kernel"`
	Uptime               string `json:"uptime"`
	Load                 string `json:"load"`
	IP                   string `json:"ip"`
	LastUpdated          string `json:"lastUpdated"`
	Processes            int    `json:"processes"`
	Connections          int    `json:"connections"`
	ConnectionsBreakdown string `json:"connectionsBreakdown"`
	CPUTemp              string `json:"cpuTemp"`
	TopProcess           string `json:"topProcess"`
}

// ResourceGauge 表示资源仪表盘中的单项指标
type ResourceGauge struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	UsedPct    float64 `json:"usedPct"`
	UsedLabel  string  `json:"usedLabel"`
	TotalLabel string  `json:"totalLabel"`
	SubLabel   string  `json:"subLabel"`
	Trend      string  `json:"trend"`
	Tone       string  `json:"tone,omitempty"`
}

// Volume 表示磁盘分区的使用情况
type Volume struct {
	Mount   string  `json:"mount"`
	UsedPct float64 `json:"usedPct"`
	Used    string  `json:"used"`
	Total   string  `json:"total"`
}

// Process 表示进程资源详情
type Process struct {
	PID     int32    `json:"pid"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	User    string   `json:"user"`
	Status  string   `json:"status"`
	CPU     float64  `json:"cpu"`
	Mem     float64  `json:"mem"`
	RSS     string   `json:"rss"`
	Threads int32    `json:"threads"`
	Start   string   `json:"start"`
	Uptime  string   `json:"uptime"`
	Ports   []string `json:"ports"`
	IORead  string   `json:"ioRead"`
	IOWrite string   `json:"ioWrite"`
	NetIn   string   `json:"netIn"`
	NetOut  string   `json:"netOut"`
	Cwd     string   `json:"cwd"`
	Path    string   `json:"path"`
	Env     []string `json:"env"`
	Note    string   `json:"note,omitempty"`
}

// SystemDashboard 聚合系统资源面板所需数据
type SystemDashboard struct {
	SystemOverview  Overview        `json:"systemOverview"`
	SystemGauges    []ResourceGauge `json:"systemGauges"`
	SystemVolumes   []Volume        `json:"systemVolumes"`
	SystemProcesses []Process       `json:"systemProcesses"`
}
