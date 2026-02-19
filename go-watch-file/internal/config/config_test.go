// 本文件用于配置加载的单元测试
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

func TestValidateConfig_InvalidFields(t *testing.T) {
	newValidConfig := func(t *testing.T) *models.Config {
		t.Helper()
		watchDir := filepath.ToSlash(t.TempDir())
		return &models.Config{
			WatchDir: watchDir,
			FileExt:  ".log",
			Bucket:   "test-bucket",
			AK:       "test-ak",
			SK:       "test-sk",
			Endpoint: "https://test-endpoint.com",
			Region:   "test-region",
			LogLevel: "info",
			APIBind:  ":8080",
		}
	}

	t.Run("missing watch dir", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.WatchDir = ""
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("missing watch dir should be allowed: %v", err)
		}
	})

	t.Run("watch dir not exists", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.WatchDir = filepath.Join(t.TempDir(), "missing")
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("监控目录不存在应该报错")
		}
	})

	t.Run("watch dir is file", func(t *testing.T) {
		cfg := newValidConfig(t)
		tempFile := filepath.Join(t.TempDir(), "file.txt")
		if err := os.WriteFile(tempFile, []byte("data"), 0o644); err != nil {
			t.Fatalf("写入文件失败: %v", err)
		}
		cfg.WatchDir = tempFile
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("监控目录为文件应该报错")
		}
	})

	t.Run("missing bucket", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.Bucket = ""
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("缺少 bucket 应该报错")
		}
	})

	t.Run("missing ak sk", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.AK = ""
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("缺少 AK/SK 应该报错")
		}
	})

	t.Run("invalid endpoint", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.Endpoint = "http://%"
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("无效 endpoint 应该报错")
		}
	})

	t.Run("missing region", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.Region = ""
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("缺少 region 应该报错")
		}
	})

	t.Run("missing api bind", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.APIBind = ""
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("缺少 API 监听地址应该报错")
		}
	})

	t.Run("alert enabled missing rules file", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.AlertEnabled = true
		cfg.AlertLogPaths = "/var/log/app/error.log"
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("告警规则为空应该报错")
		}
	})

	t.Run("alert enabled missing log paths", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.AlertRules = &models.AlertRuleset{
			Version: 1,
			Rules: []models.AlertRule{
				{
					ID:       "test",
					Level:    "ignore",
					Keywords: []string{"x"},
				},
			},
		}
		cfg.AlertEnabled = true
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("告警日志路径为空应该报错")
		}
	})

	t.Run("alert enabled invalid poll interval", func(t *testing.T) {
		cfg := newValidConfig(t)
		cfg.AlertRules = &models.AlertRuleset{
			Version: 1,
			Rules: []models.AlertRule{
				{
					ID:       "test",
					Level:    "ignore",
					Keywords: []string{"x"},
				},
			},
		}
		cfg.AlertEnabled = true
		cfg.AlertLogPaths = "/var/log/app/error.log"
		cfg.AlertPollInterval = "0s"
		if err := ValidateConfig(cfg); err == nil {
			t.Fatal("告警轮询间隔无效应该报错")
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
	if config.SystemResourceEnabled != false {
		t.Errorf("SystemResourceEnabled 默认值应为 false, 实际 %v", config.SystemResourceEnabled)
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
	if config.UploadQueuePersistEnabled {
		t.Errorf("UploadQueuePersistEnabled 默认值期望 false, 实际 %v", config.UploadQueuePersistEnabled)
	}
	if config.UploadQueuePersistFile != "" {
		t.Errorf("UploadQueuePersistFile 默认值应为空, 实际 %s", config.UploadQueuePersistFile)
	}
}

func TestLoadConfigWithPersistQueueDefaultFile(t *testing.T) {
	watchDir := filepath.ToSlash(t.TempDir())
	cfgContent := fmt.Sprintf(`
watch_dir: "%s"
file_ext: ".hprof"
bucket: "test-bucket"
ak: "test-ak"
sk: "test-sk"
endpoint: "https://test-endpoint.com"
region: "test-region"
upload_queue_persist_enabled: true
upload_queue_persist_file: ""
`, watchDir)

	configPath := writeTempConfig(t, cfgContent)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if !config.UploadQueuePersistEnabled {
		t.Fatalf("UploadQueuePersistEnabled 应该为 true")
	}
	if config.UploadQueuePersistFile != "logs/upload-queue.json" {
		t.Fatalf("UploadQueuePersistFile 默认值期望 logs/upload-queue.json, 实际 %s", config.UploadQueuePersistFile)
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	fileWatchDir := filepath.ToSlash(t.TempDir())
	envWatchDir := filepath.ToSlash(t.TempDir())

	baseConfig := fmt.Sprintf(`
watch_dir: "%s"
watch_exclude: ".git,node_modules"
file_ext: ".hprof"
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
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("ALERT_ENABLED", "true")
	t.Setenv("OSS_BUCKET", "env-bucket")
	t.Setenv("OSS_ENDPOINT", "https://env-endpoint.com")
	t.Setenv("OSS_REGION", "env-region")
	t.Setenv("OSS_FORCE_PATH_STYLE", "true")
	t.Setenv("OSS_DISABLE_SSL", "true")
	t.Setenv("OSS_AK", "env-ak")
	t.Setenv("OSS_SK", "env-sk")
	t.Setenv("DINGTALK_SECRET", "env-secret")

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	if config.WatchDir != fileWatchDir {
		t.Errorf("WatchDir should keep file value, got %s", config.WatchDir)
	}
	if config.WatchExclude != ".git,node_modules" {
		t.Errorf("WatchExclude should keep file value, got %s", config.WatchExclude)
	}
	if config.LogLevel != "info" {
		t.Errorf("LogLevel should keep file value, got %s", config.LogLevel)
	}
	if config.AlertEnabled != false {
		t.Errorf("AlertEnabled should not be overridden by env, got %v", config.AlertEnabled)
	}
	if config.Bucket != "env-bucket" {
		t.Errorf("Bucket should be overridden by env, got %s", config.Bucket)
	}
	if config.Endpoint != "https://env-endpoint.com" {
		t.Errorf("Endpoint should be overridden by env, got %s", config.Endpoint)
	}
	if config.Region != "env-region" {
		t.Errorf("Region should be overridden by env, got %s", config.Region)
	}
	if config.ForcePathStyle != true {
		t.Errorf("ForcePathStyle should be overridden by env, got %v", config.ForcePathStyle)
	}
	if config.DisableSSL != true {
		t.Errorf("DisableSSL should be overridden by env, got %v", config.DisableSSL)
	}
	if config.AK != "env-ak" || config.SK != "env-sk" {
		t.Errorf("AK/SK should be overridden by env, got ak=%s sk=%s", config.AK, config.SK)
	}
	if config.DingTalkSecret != "env-secret" {
		t.Errorf("DingTalkSecret should be overridden by env, got %s", config.DingTalkSecret)
	}
}

func TestLoadConfigRuntimeOverrides(t *testing.T) {
	baseWatchDir := filepath.ToSlash(t.TempDir())
	runtimeWatchDir := filepath.ToSlash(t.TempDir())

	baseConfig := fmt.Sprintf(`
watch_dir: "%s"
file_ext: ".hprof"
bucket: "test-bucket"
ak: "test-ak"
sk: "test-sk"
endpoint: "https://test-endpoint.com"
region: "test-region"
log_level: "info"
`, baseWatchDir)

	configPath := writeTempConfig(t, baseConfig)
	runtimePath := runtimeConfigPath(configPath)
	if runtimePath == "" {
		t.Fatal("runtime config path is empty")
	}
	runtimeContent := fmt.Sprintf(`
watch_dir: "%s"
file_ext: ".log"
`, runtimeWatchDir)
	if err := os.WriteFile(runtimePath, []byte(runtimeContent), 0o644); err != nil {
		t.Fatalf("write runtime config failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(runtimePath) })

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if config.WatchDir != runtimeWatchDir {
		t.Errorf("WatchDir should come from runtime config, got %s", config.WatchDir)
	}
	if config.FileExt != ".log" {
		t.Errorf("FileExt should come from runtime config, got %s", config.FileExt)
	}
}

func TestStringFromEnv_Trims(t *testing.T) {
	os.Setenv("TEST_STR", "  /tmp/dir  ")
	defer os.Unsetenv("TEST_STR")
	got := stringFromEnv("TEST_STR", "fallback")
	if got != "/tmp/dir" {
		t.Fatalf("期望 '/tmp/dir'，实际 '%s'", got)
	}
	os.Setenv("TEST_EMPTY", "   ")
	defer os.Unsetenv("TEST_EMPTY")
	gotEmpty := stringFromEnv("TEST_EMPTY", "fallback")
	if gotEmpty != "fallback" {
		t.Fatalf("期望 'fallback'，实际 '%s'", gotEmpty)
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
	os.Setenv("INT_EMPTY", "   ")
	defer os.Unsetenv("INT_EMPTY")
	_, ok, err = intFromEnv("INT_EMPTY")
	if err != nil || ok {
		t.Fatalf("期望空值时返回 ok=false, err=nil")
	}
}

func TestBoolFromEnv_Trims(t *testing.T) {
	os.Setenv("BOOL_KEY", " true ")
	defer os.Unsetenv("BOOL_KEY")
	v, ok, err := boolFromEnv("BOOL_KEY")
	if err != nil || !ok || v != true {
		t.Fatalf("期望 true, true, nil；实际 %v, %v, %v", v, ok, err)
	}
	os.Setenv("BOOL_EMPTY", "   ")
	defer os.Unsetenv("BOOL_EMPTY")
	_, ok, err = boolFromEnv("BOOL_EMPTY")
	if err != nil || ok {
		t.Fatalf("期望空值时返回 ok=false, err=nil")
	}
}

func TestApplyEnvOverrides_EnvOverrides(t *testing.T) {
	t.Setenv("OSS_BUCKET", "env-bucket")
	t.Setenv("OSS_ENDPOINT", "https://env-endpoint.com")
	t.Setenv("OSS_REGION", "env-region")
	t.Setenv("OSS_FORCE_PATH_STYLE", "true")
	t.Setenv("OSS_DISABLE_SSL", "true")
	t.Setenv("OSS_AK", "env-ak")
	t.Setenv("OSS_SK", "env-sk")
	t.Setenv("EMAIL_PASS", "env-pass")
	t.Setenv("UPLOAD_QUEUE_PERSIST_ENABLED", "true")
	t.Setenv("UPLOAD_QUEUE_PERSIST_FILE", "logs/persist-queue.json")
	t.Setenv("UPLOAD_ETAG_VERIFY_ENABLED", "true")
	t.Setenv("UPLOAD_RESUMABLE_ENABLED", "true")
	t.Setenv("UPLOAD_RESUMABLE_PART_SIZE", "2097152")
	t.Setenv("UPLOAD_RESUMABLE_ROUTINES", "3")
	t.Setenv("UPLOAD_RESUMABLE_THRESHOLD", "1048576")
	t.Setenv("UPLOAD_RESUMABLE_CHECKPOINT_DIR", "logs/upload-cp")
	cfg := &models.Config{
		Bucket:                    "file-bucket",
		Endpoint:                  "https://file-endpoint.com",
		Region:                    "file-region",
		ForcePathStyle:            false,
		DisableSSL:                false,
		AK:                        "file-ak",
		SK:                        "file-sk",
		EmailPass:                 "file-pass",
		UploadQueuePersistEnabled: false,
		UploadQueuePersistFile:    "",
		UploadETagVerifyEnabled:   false,
		UploadResumableEnabled:    false,
		UploadResumablePartSize:   0,
		UploadResumableRoutines:   0,
		UploadResumableThreshold:  0,
	}
	if err := applyEnvOverrides(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Bucket != "env-bucket" {
		t.Fatalf("Bucket should be overridden by env, got %s", cfg.Bucket)
	}
	if cfg.Endpoint != "https://env-endpoint.com" {
		t.Fatalf("Endpoint should be overridden by env, got %s", cfg.Endpoint)
	}
	if cfg.Region != "env-region" {
		t.Fatalf("Region should be overridden by env, got %s", cfg.Region)
	}
	if cfg.ForcePathStyle != true {
		t.Fatalf("ForcePathStyle should be overridden by env, got %v", cfg.ForcePathStyle)
	}
	if cfg.DisableSSL != true {
		t.Fatalf("DisableSSL should be overridden by env, got %v", cfg.DisableSSL)
	}
	if cfg.AK != "env-ak" || cfg.SK != "env-sk" {
		t.Fatalf("AK/SK should be overridden by env, got ak=%s sk=%s", cfg.AK, cfg.SK)
	}
	if cfg.EmailPass != "env-pass" {
		t.Fatalf("EmailPass should be overridden by env, got %s", cfg.EmailPass)
	}
	if !cfg.UploadQueuePersistEnabled {
		t.Fatalf("UploadQueuePersistEnabled should be overridden by env, got %v", cfg.UploadQueuePersistEnabled)
	}
	if cfg.UploadQueuePersistFile != "logs/persist-queue.json" {
		t.Fatalf("UploadQueuePersistFile should be overridden by env, got %s", cfg.UploadQueuePersistFile)
	}
	if !cfg.UploadETagVerifyEnabled {
		t.Fatalf("UploadETagVerifyEnabled should be overridden by env, got %v", cfg.UploadETagVerifyEnabled)
	}
	if !cfg.UploadResumableEnabled {
		t.Fatalf("UploadResumableEnabled should be overridden by env, got %v", cfg.UploadResumableEnabled)
	}
	if cfg.UploadResumablePartSize != 2097152 {
		t.Fatalf("UploadResumablePartSize should be overridden by env, got %d", cfg.UploadResumablePartSize)
	}
	if cfg.UploadResumableRoutines != 3 {
		t.Fatalf("UploadResumableRoutines should be overridden by env, got %d", cfg.UploadResumableRoutines)
	}
	if cfg.UploadResumableThreshold != 1048576 {
		t.Fatalf("UploadResumableThreshold should be overridden by env, got %d", cfg.UploadResumableThreshold)
	}
	if cfg.UploadResumableCheckpointDir != "logs/upload-cp" {
		t.Fatalf("UploadResumableCheckpointDir should be overridden by env, got %s", cfg.UploadResumableCheckpointDir)
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
