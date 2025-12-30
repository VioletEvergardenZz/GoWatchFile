package state

// 定义后端返回给前端的 JSON 结构（DTO）

// FileNode 描述控制台 UI 的目录树节点
type FileNode struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Type       string     `json:"type"` // 节点类型: "dir" | "file"
	AutoUpload bool       `json:"autoUpload"`
	Size       string     `json:"size,omitempty"`
	Updated    string     `json:"updated,omitempty"`
	Content    string     `json:"content,omitempty"`
	Children   []FileNode `json:"children,omitempty"`
}

// MetricCard 表示一个小型指标展示块
type MetricCard struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Trend string `json:"trend"`
	Tone  string `json:"tone,omitempty"`
}

// FileItem 表示文件列表中的一行
type FileItem struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Size             string `json:"size"`
	Status           string `json:"status"` // 状态: uploaded | queued | failed | existing
	Time             string `json:"time"`
	AutoUpload       bool   `json:"autoUpload"`
}

// TimelineEvent 表示时间线中的一个事件
type TimelineEvent struct {
	Label  string `json:"label"`
	Time   string `json:"time"`
	Status string `json:"status"` // 状态: info | success | warning | danger
	Host   string `json:"host,omitempty"`
}

// MonitorNote 表示一个小型说明块
type MonitorNote struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// ConfigSnapshot 对应 UI 可编辑配置表单的快照
type ConfigSnapshot struct {
	WatchDir    string `json:"watchDir"`
	FileExt     string `json:"fileExt"`
	Silence     string `json:"silence"`
	Concurrency string `json:"concurrency"`
}

// HeroCopy 表示首页头部摘要信息
type HeroCopy struct {
	Agent        string   `json:"agent"`
	WatchDirs    []string `json:"watchDirs"`
	SuffixFilter string   `json:"suffixFilter"`
	Silence      string   `json:"silence"`
	Queue        string   `json:"queue"`
	Concurrency  string   `json:"concurrency"`
}

// ChartPoint 表示折线图中的一个点
type ChartPoint struct {
	Label    string `json:"label"`
	Uploads  int    `json:"uploads"`
	Failures int    `json:"failures"`
	Queue    int    `json:"queue"`			//队列长度
}

// UploadRecord 表示最近的上传记录
type UploadRecord struct {
	File    string `json:"file"`
	Target  string `json:"target"` // 下载地址
	Size    string `json:"size"`
	Result  string `json:"result"` // 结果: success | failed | pending
	Latency string `json:"latency"`
	Time    string `json:"time"`
	Note    string `json:"note,omitempty"`
}

// MonitorSummary 表示摘要指标项
type MonitorSummary struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Desc  string `json:"desc"`
}

// DashboardData 是前端需要的聚合数据载体
type DashboardData struct {
	HeroCopy       HeroCopy         `json:"heroCopy"`
	MetricCards    []MetricCard     `json:"metricCards"`
	DirectoryTree  []FileNode       `json:"directoryTree"`
	Files          []FileItem       `json:"files"`
	TailLines      []string         `json:"tailLines"`
	TimelineEvents []TimelineEvent  `json:"timelineEvents"`
	MonitorNotes   []MonitorNote    `json:"monitorNotes"`
	UploadRecords  []UploadRecord   `json:"uploadRecords"`
	MonitorSummary []MonitorSummary `json:"monitorSummary"`
	ConfigSnapshot ConfigSnapshot   `json:"configSnapshot"`
	ChartPoints    []ChartPoint     `json:"chartPoints"`
}
