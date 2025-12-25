package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"file-watch/internal/models"
)

func TestLoadConfig(t *testing.T) {
	watchDir := filepath.ToSlash(t.TempDir())

	tempConfig := fmt.Sprintf(`
watch_dir: "%s"
file_ext: ".hprof"
robot_key: "test-key"
dingtalk_webhook: "https://oapi.dingtalk.com/robot/send?access_token=test-token"
dingtalk_secret: "test-secret"
bucket: "test-bucket"
ak: "test-ak"
sk: "test-sk"
endpoint: "https://test-endpoint.com"
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
`, watchDir)

	configPath := writeTempConfig(t, tempConfig)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config.WatchDir != watchDir {
		t.Errorf("WatchDir 期望 %s, 实际 %s", watchDir, config.WatchDir)
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
	t.Run("valid config", func(t *testing.T) {
		watchDir := filepath.ToSlash(t.TempDir())
		validConfig := &models.Config{
			WatchDir:    watchDir,
			FileExt:     ".hprof",
			RobotKey:    "test-key",
			Bucket:      "test-bucket",
			AK:          "test-ak",
			SK:          "test-sk",
			Endpoint:    "https://test-endpoint.com",
			Region:      "test-region",
			JenkinsHost: "http://test-jenkins.com",
			JenkinsJob:  "test-job",
			LogLevel:    "info",
		}

		if err := ValidateConfig(validConfig); err != nil {
			t.Fatalf("有效配置验证失败: %v", err)
		}
	})

	t.Run("invalid file ext", func(t *testing.T) {
		watchDir := filepath.ToSlash(t.TempDir())
		invalidConfig := &models.Config{
			WatchDir:    watchDir,
			FileExt:     "hprof", // missing leading dot
			Bucket:      "test-bucket",
			AK:          "test-ak",
			SK:          "test-sk",
			Endpoint:    "https://test-endpoint.com",
			Region:      "test-region",
			JenkinsHost: "http://test-jenkins.com",
			JenkinsJob:  "test-job",
			LogLevel:    "info",
		}

		if err := ValidateConfig(invalidConfig); err == nil {
			t.Fatal("无效配置应该验证失败")
		}
	})

	t.Run("invalid log level", func(t *testing.T) {
		watchDir := filepath.ToSlash(t.TempDir())
		invalidConfig := &models.Config{
			WatchDir:    watchDir,
			FileExt:     ".hprof",
			Bucket:      "test-bucket",
			AK:          "test-ak",
			SK:          "test-sk",
			Endpoint:    "https://test-endpoint.com",
			Region:      "test-region",
			JenkinsHost: "http://test-jenkins.com",
			JenkinsJob:  "test-job",
			LogLevel:    "infos",
		}

		if err := ValidateConfig(invalidConfig); err == nil {
			t.Fatal("无效日志级别应该验证失败")
		}
	})
}

func TestLoadConfigWithDefaults(t *testing.T) {
	watchDir := filepath.ToSlash(t.TempDir())
	minimalConfig := fmt.Sprintf(`
watch_dir: "%s"
file_ext: ".hprof"
robot_key: "test-key"
bucket: "test-bucket"
ak: "test-ak"
sk: "test-sk"
endpoint: "https://test-endpoint.com"
region: "test-region"
jenkins_host: "http://test-jenkins.com"
jenkins_job: "test-job"
`, watchDir)

	configPath := writeTempConfig(t, minimalConfig)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

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

func TestLoadConfigEnvOverrides(t *testing.T) {
	fileWatchDir := filepath.ToSlash(t.TempDir())
	envWatchDir := filepath.ToSlash(t.TempDir())

	baseConfig := fmt.Sprintf(`
watch_dir: "%s"
file_ext: ".hprof"
robot_key: "test-key"
bucket: "test-bucket"
ak: "file-ak"
sk: "file-sk"
endpoint: "https://test-endpoint.com"
region: "test-region"
jenkins_host: "http://test-jenkins.com"
jenkins_job: "test-job"
upload_workers: 2
log_level: "info"
`, fileWatchDir)

	configPath := writeTempConfig(t, baseConfig)

	t.Setenv("WATCH_DIR", envWatchDir)
	t.Setenv("S3_AK", "env-ak")
	t.Setenv("S3_SK", "env-sk")
	t.Setenv("UPLOAD_WORKERS", "7")
	t.Setenv("LOG_LEVEL", "error")

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config.WatchDir != envWatchDir {
		t.Errorf("WatchDir 应从环境变量覆盖, 实际 %s", config.WatchDir)
	}
	if config.AK != "env-ak" || config.SK != "env-sk" {
		t.Errorf("AK/SK 应从环境变量覆盖, 实际 ak=%s sk=%s", config.AK, config.SK)
	}
	if config.UploadWorkers != 7 {
		t.Errorf("UploadWorkers 应从环境变量覆盖为 7, 实际 %d", config.UploadWorkers)
	}
	if config.LogLevel != "error" {
		t.Errorf("LogLevel 应从环境变量覆盖为 error, 实际 %s", config.LogLevel)
	}
}

func TestStringFromEnv_Trims(t *testing.T) {
	os.Setenv("TEST_STR", "  /tmp/dir  ")
	defer os.Unsetenv("TEST_STR")
	got := stringFromEnv("TEST_STR", "fallback")
	if got != "/tmp/dir" {
		t.Fatalf("expected '/tmp/dir', got '%s'", got)
	}
}

func TestResolveEnvPlaceholder(t *testing.T) {
	os.Setenv("PLACE", " value ")
	defer os.Unsetenv("PLACE")
	got := resolveEnvPlaceholder("${PLACE}")
	if got != "value" {
		t.Fatalf("expected 'value', got '%s'", got)
	}
	got2 := resolveEnvPlaceholder("${MISSING}")
	if got2 != "" {
		t.Fatalf("expected '', got '%s'", got2)
	}
}

func TestIntFromEnv(t *testing.T) {
	os.Setenv("INT_KEY", "  42 ")
	defer os.Unsetenv("INT_KEY")
	v, ok, err := intFromEnv("INT_KEY")
	if err != nil || !ok || v != 42 {
		t.Fatalf("expected 42, true, nil; got %v, %v, %v", v, ok, err)
	}
	os.Setenv("INT_BAD", "notint")
	defer os.Unsetenv("INT_BAD")
	_, _, err = intFromEnv("INT_BAD")
	if err == nil {
		t.Fatalf("expected error for invalid int")
	}
}

func TestBoolFromEnv_Trims(t *testing.T) {
	os.Setenv("BOOL_KEY", " true ")
	defer os.Unsetenv("BOOL_KEY")
	v, ok, err := boolFromEnv("BOOL_KEY")
	if err != nil || !ok || v != true {
		t.Fatalf("expected true, true, nil; got %v, %v, %v", v, ok, err)
	}
}

func TestApplyEnvOverrides_UploadWorkers(t *testing.T) {
	os.Setenv("UPLOAD_WORKERS", "7")
	defer os.Unsetenv("UPLOAD_WORKERS")
	cfg := &models.Config{}
	if err := applyEnvOverrides(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UploadWorkers != 7 {
		t.Fatalf("expected UploadWorkers=7, got %d", cfg.UploadWorkers)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("关闭临时文件失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })
	return tmpFile.Name()
}
