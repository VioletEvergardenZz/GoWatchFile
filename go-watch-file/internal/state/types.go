package state

// FileNode describes a directory tree node for the console UI.
type FileNode struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Type       string     `json:"type"` // "dir" | "file"
	AutoUpload bool       `json:"autoUpload"`
	Size       string     `json:"size,omitempty"`
	Updated    string     `json:"updated,omitempty"`
	Content    string     `json:"content,omitempty"`
	Children   []FileNode `json:"children,omitempty"`
}

// MetricCard is a small metric display block.
type MetricCard struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Trend string `json:"trend"`
	Tone  string `json:"tone,omitempty"`
}

// FileItem represents a row in the file list.
type FileItem struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Size             string `json:"size"`
	Status           string `json:"status"` // uploaded | queued | failed | existing
	Time             string `json:"time"`
	AutoUpload       bool   `json:"autoUpload"`
	RequiresApproval bool   `json:"requiresApproval,omitempty"`
}

// TimelineEvent is a simple event in the timeline.
type TimelineEvent struct {
	Label  string `json:"label"`
	Time   string `json:"time"`
	Status string `json:"status"` // info | success | warning | danger
	Host   string `json:"host,omitempty"`
}

// RoutePreview renders quick routing rule hints.
type RoutePreview struct {
	Name   string `json:"name"`
	Cond   string `json:"cond"`
	Action string `json:"action"`
}

// MonitorNote is a small informational block.
type MonitorNote struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// ConfigSnapshot mirrors the editable config form in UI.
type ConfigSnapshot struct {
	WatchDir    string `json:"watchDir"`
	FileExt     string `json:"fileExt"`
	Silence     string `json:"silence"`
	Concurrency string `json:"concurrency"`
	Bucket      string `json:"bucket"`
	Action      string `json:"action"`
}

// HeroCopy shows the hero section summary.
type HeroCopy struct {
	Agent        string   `json:"agent"`
	WatchDirs    []string `json:"watchDirs"`
	SuffixFilter string   `json:"suffixFilter"`
	Silence      string   `json:"silence"`
	Queue        string   `json:"queue"`
	Concurrency  string   `json:"concurrency"`
	Bucket       string   `json:"bucket"`
}

// ChartPoint drives the line chart.
type ChartPoint struct {
	Label    string `json:"label"`
	Uploads  int    `json:"uploads"`
	Failures int    `json:"failures"`
	Queue    int    `json:"queue"`
}

// UploadRecord shows recent upload actions.
type UploadRecord struct {
	File    string `json:"file"`
	Target  string `json:"target"`
	Size    string `json:"size"`
	Result  string `json:"result"` // success | failed | pending
	Latency string `json:"latency"`
	Time    string `json:"time"`
	Note    string `json:"note,omitempty"`
}

// MonitorSummary shows small summary metrics.
type MonitorSummary struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Desc  string `json:"desc"`
}

// DashboardData is the aggregated payload the frontend expects.
type DashboardData struct {
	HeroCopy       HeroCopy         `json:"heroCopy"`
	HeroHighlights []string         `json:"heroHighlights"`
	MetricCards    []MetricCard     `json:"metricCards"`
	DirectoryTree  []FileNode       `json:"directoryTree"`
	Files          []FileItem       `json:"files"`
	TailLines      []string         `json:"tailLines"`
	TimelineEvents []TimelineEvent  `json:"timelineEvents"`
	Routes         []RoutePreview   `json:"routes"`
	MonitorNotes   []MonitorNote    `json:"monitorNotes"`
	UploadRecords  []UploadRecord   `json:"uploadRecords"`
	MonitorSummary []MonitorSummary `json:"monitorSummary"`
	ConfigSnapshot ConfigSnapshot   `json:"configSnapshot"`
	ChartPoints    []ChartPoint     `json:"chartPoints"`
}
