package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"file-watch/internal/models"
)

// 覆盖配置加载流程
func TestLoadConfig(t *testing.T) {
	watchDir := filepath.ToSlash(t.TempDir())

	tempConfig := fmt.Sprintf(`
watch_dir: "%s"
watch_exclude: ".git,node_modules"
file_ext: ".hprof"
robot_key: "test-key"
dingtalk_webhook: "https://oapi.dingtalk.com/robot/send?access_token=test-token"
dingtalk_secret: "test-secret"
email_host: "smtp.example.com"
email_port: 587
email_user: "user@example.com"
email_pass: "passw0rd"
email_from: "alerts@example.com"
email_to: "ops@example.com,dev@example.com"
email_use_tls: true
bucket: "test-bucket"
ak: "test-ak"
sk: "test-sk"
endpoint: "https://test-endpoint.com"
region: "test-region"
force_path_style: true
disable_ssl: false
log_level: "debug"
log_file: "/var/log/test.log"
log_to_std: false
log_show_caller: true
upload_workers: 5
upload_queue_size: 200
api_bind: ":9000"
`, watchDir)

	configPath := writeTempConfig(t, tempConfig)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config.WatchDir != watchDir {
		t.Errorf("WatchDir 期望 %s, 实际 %s", watchDir, config.WatchDir)
	}
	if config.WatchExclude != ".git,node_modules" {
		t.Errorf("WatchExclude 期望 .git,node_modules, 实际 %s", config.WatchExclude)
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
	if config.EmailHost != "smtp.example.com" {
		t.Errorf("EmailHost 期望 smtp.example.com, 实际 %s", config.EmailHost)
	}
	if config.EmailPort != 587 {
		t.Errorf("EmailPort 期望 587, 实际 %d", config.EmailPort)
	}
	if config.EmailUser != "user@example.com" {
		t.Errorf("EmailUser 期望 user@example.com, 实际 %s", config.EmailUser)
	}
	if config.EmailPass != "passw0rd" {
		t.Errorf("EmailPass 期望 passw0rd, 实际 %s", config.EmailPass)
	}
	if config.EmailFrom != "alerts@example.com" {
		t.Errorf("EmailFrom 期望 alerts@example.com, 实际 %s", config.EmailFrom)
	}
	if config.EmailTo != "ops@example.com,dev@example.com" {
		t.Errorf("EmailTo 期望 ops@example.com,dev@example.com, 实际 %s", config.EmailTo)
	}
	if config.EmailUseTLS != true {
		t.Errorf("EmailUseTLS 期望 true, 实际 %v", config.EmailUseTLS)
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
	if config.APIBind != ":9000" {
		t.Errorf("APIBind 期望 :9000, 实际 %s", config.APIBind)
	}
}

func TestValidateConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		watchDir := filepath.ToSlash(t.TempDir())
		validConfig := &models.Config{
			WatchDir: watchDir,
			FileExt:  ".hprof",
			RobotKey: "test-key",
			Bucket:   "test-bucket",
			AK:       "test-ak",
			SK:       "test-sk",
			Endpoint: "https://test-endpoint.com",
			Region:   "test-region",
			LogLevel: "info",
			APIBind:  ":8080",
		}

		if err := ValidateConfig(validConfig); err != nil {
			t.Fatalf("有效配置验证失败: %v", err)
		}
	})

	t.Run("invalid file ext", func(t *testing.T) {
		watchDir := filepath.ToSlash(t.TempDir())
		invalidConfig := &models.Config{
			WatchDir: watchDir,
			FileExt:  "hprof", // 缺少前导点
			Bucket:   "test-bucket",
			AK:       "test-ak",
			SK:       "test-sk",
			Endpoint: "https://test-endpoint.com",
			Region:   "test-region",
			LogLevel: "info",
			APIBind:  ":8080",
		}

		if err := ValidateConfig(invalidConfig); err == nil {
			t.Fatal("无效配置应该验证失败")
		}
	})

	t.Run("multi watch dir", func(t *testing.T) {
		watchDir1 := filepath.ToSlash(t.TempDir())
		watchDir2 := filepath.ToSlash(t.TempDir())
		validConfig := &models.Config{
			WatchDir: watchDir1 + "," + watchDir2,
			FileExt:  ".log",
			Bucket:   "test-bucket",
			AK:       "test-ak",
			SK:       "test-sk",
			Endpoint: "https://test-endpoint.com",
			Region:   "test-region",
			LogLevel: "info",
			APIBind:  ":8080",
		}
		if err := ValidateConfig(validConfig); err != nil {
			t.Fatalf("多目录配置验证失败: %v", err)
		}
	})

	t.Run("multi file ext", func(t *testing.T) {
		watchDir := filepath.ToSlash(t.TempDir())
		validConfig := &models.Config{
			WatchDir: watchDir,
			FileExt:  ".log, .txt",
			Bucket:   "test-bucket",
			AK:       "test-ak",
			SK:       "test-sk",
			Endpoint: "https://test-endpoint.com",
			Region:   "test-region",
			LogLevel: "info",
			APIBind:  ":8080",
		}
		if err := ValidateConfig(validConfig); err != nil {
			t.Fatalf("多后缀配置验证失败: %v", err)
		}
	})

	t.Run("invalid log level", func(t *testing.T) {
		watchDir := filepath.ToSlash(t.TempDir())
		invalidConfig := &models.Config{
			WatchDir: watchDir,
			FileExt:  ".hprof",
			Bucket:   "test-bucket",
			AK:       "test-ak",
			SK:       "test-sk",
			Endpoint: "https://test-endpoint.com",
			Region:   "test-region",
			LogLevel: "infos",
			APIBind:  ":8080",
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
	if config.APIBind != ":8080" {
		t.Errorf("APIBind 默认值期望 :8080, 实际 %s", config.APIBind)
	}
	if config.AlertPollInterval != "2s" {
		t.Errorf("AlertPollInterval 默认值期望 2s, 实际 %s", config.AlertPollInterval)
	}
	if config.AlertSuppressEnabled == nil || *config.AlertSuppressEnabled != true {
		t.Errorf("AlertSuppressEnabled 默认值期望 true, 实际 %v", config.AlertSuppressEnabled)
	}
	if config.AlertStartFromEnd == nil || *config.AlertStartFromEnd != true {
		t.Errorf("AlertStartFromEnd 默认值期望 true, 实际 %v", config.AlertStartFromEnd)
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	fileWatchDir := filepath.ToSlash(t.TempDir())
	envWatchDir := filepath.ToSlash(t.TempDir())
	alertRulePath := filepath.ToSlash(filepath.Join(t.TempDir(), "alert-rules.yaml"))
	if err := os.WriteFile(alertRulePath, []byte("version: 1\nrules:\n  - id: test\n    level: ignore\n    keywords: [\"test\"]\n"), 0644); err != nil {
		t.Fatalf("写入临时规则文件失败: %v", err)
	}

	baseConfig := fmt.Sprintf(`
watch_dir: "%s"
file_ext: ".hprof"
robot_key: "test-key"
bucket: "test-bucket"
ak: "file-ak"
sk: "file-sk"
endpoint: "https://test-endpoint.com"
region: "test-region"
upload_workers: 2
log_level: "info"
`, fileWatchDir)

	configPath := writeTempConfig(t, baseConfig)

	t.Setenv("WATCH_DIR", envWatchDir)
	t.Setenv("WATCH_EXCLUDE", ".git,.cache")
	t.Setenv("S3_AK", "env-ak")
	t.Setenv("S3_SK", "env-sk")
	t.Setenv("UPLOAD_WORKERS", "7")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("API_BIND", ":18080")
	t.Setenv("ALERT_ENABLED", "true")
	t.Setenv("ALERT_SUPPRESS_ENABLED", "false")
	t.Setenv("ALERT_RULES_FILE", alertRulePath)
	t.Setenv("ALERT_LOG_PATHS", "/var/log/app/error.log")
	t.Setenv("ALERT_POLL_INTERVAL", "5s")
	t.Setenv("ALERT_START_FROM_END", "false")

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config.WatchDir != envWatchDir {
		t.Errorf("WatchDir 应从环境变量覆盖, 实际 %s", config.WatchDir)
	}
	if config.WatchExclude != ".git,.cache" {
		t.Errorf("WatchExclude 应从环境变量覆盖, 实际 %s", config.WatchExclude)
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
	if config.APIBind != ":18080" {
		t.Errorf("APIBind 应从环境变量覆盖为 :18080, 实际 %s", config.APIBind)
	}
	if config.AlertEnabled != true {
		t.Errorf("AlertEnabled 应从环境变量覆盖为 true, 实际 %v", config.AlertEnabled)
	}
	if config.AlertSuppressEnabled == nil || *config.AlertSuppressEnabled != false {
		t.Errorf("AlertSuppressEnabled 应从环境变量覆盖为 false, 实际 %v", config.AlertSuppressEnabled)
	}
	if config.AlertRulesFile != alertRulePath {
		t.Errorf("AlertRulesFile 应从环境变量覆盖, 实际 %s", config.AlertRulesFile)
	}
	if config.AlertLogPaths != "/var/log/app/error.log" {
		t.Errorf("AlertLogPaths 应从环境变量覆盖, 实际 %s", config.AlertLogPaths)
	}
	if config.AlertPollInterval != "5s" {
		t.Errorf("AlertPollInterval 应从环境变量覆盖为 5s, 实际 %s", config.AlertPollInterval)
	}
	if config.AlertStartFromEnd == nil || *config.AlertStartFromEnd != false {
		t.Errorf("AlertStartFromEnd 应从环境变量覆盖为 false, 实际 %v", config.AlertStartFromEnd)
	}
}

func TestStringFromEnv_Trims(t *testing.T) {
	os.Setenv("TEST_STR", "  /tmp/dir  ")
	defer os.Unsetenv("TEST_STR")
	got := stringFromEnv("TEST_STR", "fallback")
	if got != "/tmp/dir" {
		t.Fatalf("期望 '/tmp/dir'，实际 '%s'", got)
	}
}

func TestResolveEnvPlaceholder(t *testing.T) {
	os.Setenv("PLACE", " value ")
	defer os.Unsetenv("PLACE")
	got := resolveEnvPlaceholder("${PLACE}")
	if got != "value" {
		t.Fatalf("期望 'value'，实际 '%s'", got)
	}
	got2 := resolveEnvPlaceholder("${MISSING}")
	if got2 != "" {
		t.Fatalf("期望 ''，实际 '%s'", got2)
	}
}

func TestIntFromEnv(t *testing.T) {
	os.Setenv("INT_KEY", "  42 ")
	defer os.Unsetenv("INT_KEY")
	v, ok, err := intFromEnv("INT_KEY")
	if err != nil || !ok || v != 42 {
		t.Fatalf("期望 42, true, nil；实际 %v, %v, %v", v, ok, err)
	}
	os.Setenv("INT_BAD", "notint")
	defer os.Unsetenv("INT_BAD")
	_, _, err = intFromEnv("INT_BAD")
	if err == nil {
		t.Fatalf("期望无效整数时返回错误")
	}
}

func TestBoolFromEnv_Trims(t *testing.T) {
	os.Setenv("BOOL_KEY", " true ")
	defer os.Unsetenv("BOOL_KEY")
	v, ok, err := boolFromEnv("BOOL_KEY")
	if err != nil || !ok || v != true {
		t.Fatalf("期望 true, true, nil；实际 %v, %v, %v", v, ok, err)
	}
}

func TestApplyEnvOverrides_UploadWorkers(t *testing.T) {
	os.Setenv("UPLOAD_WORKERS", "7")
	defer os.Unsetenv("UPLOAD_WORKERS")
	cfg := &models.Config{}
	if err := applyEnvOverrides(cfg); err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if cfg.UploadWorkers != 7 {
		t.Fatalf("期望 UploadWorkers=7，实际 %d", cfg.UploadWorkers)
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
