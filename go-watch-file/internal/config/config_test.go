package config

import (
	"os"
	"testing"

	"file-watch/internal/models"
)

func TestLoadConfig(t *testing.T) {
	// 创建临时配置文件
	tempConfig := `
watch_dir: "/test/dir"
file_ext: ".hprof"
robot_key: "test-key"
dingtalk_webhook: "https://oapi.dingtalk.com/robot/send?access_token=test-token"
dingtalk_secret: "test-secret"
bucket: "test-bucket"
ak: "test-ak"
sk: "test-sk"
endpoint: "test-endpoint"
region: "test-region"
force_path_style: true
disable_ssl: false
jenkins_host: "http://test-jenkins.com"
jenkins_user: "test-user"
jenkins_password: "test-password"
jenkins_job: "test-job"
log_level: "debug"
log_file: "/var/log/test.log"
log_to_std: false
log_show_caller: true
upload_workers: 5
upload_queue_size: 200
`

	// 写入临时文件
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(tempConfig); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	tmpFile.Close()

	// 测试加载配置
	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置值
	if config.WatchDir != "/test/dir" {
		t.Errorf("WatchDir 期望 /test/dir, 实际 %s", config.WatchDir)
	}
	if config.FileExt != ".hprof" {
		t.Errorf("FileExt 期望 .hprof, 实际 %s", config.FileExt)
	}
	if config.DingTalkWebhook != "https://oapi.dingtalk.com/robot/send?access_token=test-token" {
		t.Errorf("DingTalkWebhook 期望 test-token url, 实际 %s", config.DingTalkWebhook)
	}
	if config.DingTalkSecret != "test-secret" {
		t.Errorf("DingTalkSecret 期望 test-secret, 实际 %s", config.DingTalkSecret)
	}
	if config.LogLevel != "debug" {
		t.Errorf("LogLevel 期望 debug, 实际 %s", config.LogLevel)
	}
	if config.LogToStd == nil || *config.LogToStd != false {
		t.Errorf("LogToStd 期望 false, 实际 %v", config.LogToStd)
	}
	if config.LogShowCaller != true {
		t.Errorf("LogShowCaller 期望 true, 实际 %v", config.LogShowCaller)
	}
	if config.UploadWorkers != 5 {
		t.Errorf("UploadWorkers 期望 5, 实际 %d", config.UploadWorkers)
	}
	if config.UploadQueueSize != 200 {
		t.Errorf("UploadQueueSize 期望 200, 实际 %d", config.UploadQueueSize)
	}
}

func TestValidateConfig(t *testing.T) {
	// 测试有效配置
	validConfig := &models.Config{
		WatchDir:    "/test/dir",
		FileExt:     ".hprof",
		RobotKey:    "test-key",
		Bucket:      "test-bucket",
		AK:          "test-ak",
		SK:          "test-sk",
		Endpoint:    "test-endpoint",
		Region:      "test-region",
		JenkinsHost: "http://test-jenkins.com",
		JenkinsJob:  "test-job",
	}

	if err := ValidateConfig(validConfig); err != nil {
		t.Errorf("有效配置验证失败: %v", err)
	}

	// 测试无效配置
	invalidConfig := &models.Config{
		WatchDir: "", // 空监控目录
	}

	if err := ValidateConfig(invalidConfig); err == nil {
		t.Error("无效配置应该验证失败")
	}
}

func TestLoadConfigWithDefaults(t *testing.T) {
	// 创建最小配置文件
	minimalConfig := `
watch_dir: "/test/dir"
file_ext: ".hprof"
robot_key: "test-key"
bucket: "test-bucket"
ak: "test-ak"
sk: "test-sk"
endpoint: "test-endpoint"
region: "test-region"
jenkins_host: "http://test-jenkins.com"
jenkins_job: "test-job"
`

	// 写入临时文件
	tmpFile, err := os.CreateTemp("", "test-minimal-config-*.yaml")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(minimalConfig); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	tmpFile.Close()

	// 测试加载配置
	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证默认值
	if config.UploadWorkers != 3 {
		t.Errorf("UploadWorkers 默认值期望 3, 实际 %d", config.UploadWorkers)
	}
	if config.UploadQueueSize != 100 {
		t.Errorf("UploadQueueSize 默认值期望 100, 实际 %d", config.UploadQueueSize)
	}
	if config.LogLevel != "info" {
		t.Errorf("LogLevel 默认值期望 info, 实际 %s", config.LogLevel)
	}
	if config.LogToStd == nil || *config.LogToStd != true {
		t.Errorf("LogToStd 默认值期望 true, 实际 %v", config.LogToStd)
	}
	if config.LogShowCaller != false {
		t.Errorf("LogShowCaller 默认值期望 false, 实际 %v", config.LogShowCaller)
	}
}
