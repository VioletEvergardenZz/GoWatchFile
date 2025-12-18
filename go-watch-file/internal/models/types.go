package models

import (
	"time"
)

// Config 配置结构体
type Config struct {
	WatchDir        string `yaml:"watch_dir"`
	FileExt         string `yaml:"file_ext"`
	RobotKey        string `yaml:"robot_key"`
	DingTalkWebhook string `yaml:"dingtalk_webhook"`
	DingTalkSecret  string `yaml:"dingtalk_secret"`
	Bucket          string `yaml:"bucket"`
	AK              string `yaml:"ak"`
	SK              string `yaml:"sk"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	ForcePathStyle  bool   `yaml:"force_path_style"`
	DisableSSL      bool   `yaml:"disable_ssl"`
	JenkinsHost     string `yaml:"jenkins_host"`
	JenkinsUser     string `yaml:"jenkins_user"`
	JenkinsPassword string `yaml:"jenkins_password"`
	JenkinsJob      string `yaml:"jenkins_job"`
	LogLevel        string `yaml:"log_level"`
	LogFile         string `yaml:"log_file"`
	UploadWorkers   int    `yaml:"upload_workers"`    // 上传工作池大小
	UploadQueueSize int    `yaml:"upload_queue_size"` // 上传队列大小
}

// FileEvent 文件事件结构体
type FileEvent struct {
	FilePath string
	Op       string
	Time     time.Time
}

// UploadStats 上传统计信息
type UploadStats struct {
	QueueLength int
	Workers     int
}
