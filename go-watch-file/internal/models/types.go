// 本文件用于定义配置与业务模型
package models

import (
	"time"
)

// Config 配置结构体
type Config struct {
	WatchDir                  string        `yaml:"watch_dir"`
	WatchExclude              string        `yaml:"watch_exclude"`
	FileExt                   string        `yaml:"file_ext"`
	Silence                   string        `yaml:"silence"`
	RobotKey                  string        `yaml:"robot_key"`
	DingTalkWebhook           string        `yaml:"dingtalk_webhook"`
	DingTalkSecret            string        `yaml:"dingtalk_secret"`
	EmailHost                 string        `yaml:"email_host"`
	EmailPort                 int           `yaml:"email_port"`
	EmailUser                 string        `yaml:"email_user"`
	EmailPass                 string        `yaml:"email_pass"`
	EmailFrom                 string        `yaml:"email_from"`
	EmailTo                   string        `yaml:"email_to"`
	EmailUseTLS               bool          `yaml:"email_use_tls"`
	Bucket                    string        `yaml:"bucket"`
	AK                        string        `yaml:"ak"`
	SK                        string        `yaml:"sk"`
	Endpoint                  string        `yaml:"endpoint"`
	Region                    string        `yaml:"region"`
	ForcePathStyle            bool          `yaml:"force_path_style"`
	DisableSSL                bool          `yaml:"disable_ssl"`
	LogLevel                  string        `yaml:"log_level"`
	LogFile                   string        `yaml:"log_file"`
	LogToStd                  *bool         `yaml:"log_to_std"`
	LogShowCaller             bool          `yaml:"log_show_caller"`
	APIAuthToken              string        `yaml:"api_auth_token"`
	APICORSOrigins            string        `yaml:"api_cors_origins"`
	UploadWorkers             int           `yaml:"upload_workers"`    // 上传工作池大小
	UploadQueueSize           int           `yaml:"upload_queue_size"` // 上传队列大小
	UploadQueuePersistEnabled bool          `yaml:"upload_queue_persist_enabled"`
	UploadQueuePersistFile    string        `yaml:"upload_queue_persist_file"`
	UploadRetryDelays         string        `yaml:"upload_retry_delays"`
	UploadRetryEnabled        *bool         `yaml:"upload_retry_enabled"`
	APIBind                   string        `yaml:"api_bind"` // API 服务监听地址
	SystemResourceEnabled     bool          `yaml:"system_resource_enabled"`
	AlertEnabled              bool          `yaml:"alert_enabled"`
	AlertSuppressEnabled      *bool         `yaml:"alert_suppress_enabled"`
	AlertRules                *AlertRuleset `yaml:"alert_rules"`
	AlertRulesFile            string        `yaml:"alert_rules_file"`
	AlertLogPaths             string        `yaml:"alert_log_paths"`
	AlertPollInterval         string        `yaml:"alert_poll_interval"`
	AlertStartFromEnd         *bool         `yaml:"alert_start_from_end"`
	AIEnabled                 bool          `yaml:"ai_enabled"`
	AIBaseURL                 string        `yaml:"ai_base_url"`
	AIAPIKey                  string        `yaml:"ai_api_key"`
	AIModel                   string        `yaml:"ai_model"`
	AITimeout                 string        `yaml:"ai_timeout"`
	AIMaxLines                int           `yaml:"ai_max_lines"`
}

// FileEvent 文件事件结构体
type FileEvent struct {
	FilePath string    // 文件路径
	Op       string    // 事件类型
	Time     time.Time // 事件时间
}

// UploadStats 上传统计信息
type UploadStats struct {
	QueueLength int // 上传队列长度
	Workers     int // 上传 worker 数量
	InFlight    int // 正在上传的数量
}

// FailureReasonCount 表示失败原因统计
type FailureReasonCount struct {
	Reason string `json:"reason"`
	Count  uint64 `json:"count"`
}

// PersistQueueHealth 表示上传持久化队列健康指标
type PersistQueueHealth struct {
	Enabled                  bool   `json:"enabled"`
	StoreFile                string `json:"storeFile"`
	RecoveredTotal           uint64 `json:"recoveredTotal"`
	CorruptFallbackTotal     uint64 `json:"corruptFallbackTotal"`
	PersistWriteFailureTotal uint64 `json:"persistWriteFailureTotal"`
}

// HealthSnapshot 表示健康检查返回的运行指标
type HealthSnapshot struct {
	QueueLength        int                  `json:"queue"`
	Workers            int                  `json:"workers"`
	InFlight           int                  `json:"inFlight"`
	QueueFullTotal     uint64               `json:"queueFullTotal"`
	RetryTotal         uint64               `json:"retryTotal"`
	UploadFailureTotal uint64               `json:"uploadFailureTotal"`
	FailureReasons     []FailureReasonCount `json:"failureReasons"`
	PersistQueue       PersistQueueHealth   `json:"persistQueue"`
}
