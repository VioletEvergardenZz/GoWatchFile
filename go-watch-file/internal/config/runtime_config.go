// 本文件用于控制台运行时配置的读取与持久化
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"file-watch/internal/models"
)

type runtimeConfig struct {
	WatchDir             *string `yaml:"watch_dir"`
	FileExt              *string `yaml:"file_ext"`
	Silence              *string `yaml:"silence"`
	UploadWorkers        *int    `yaml:"upload_workers"`
	UploadQueueSize      *int    `yaml:"upload_queue_size"`
	SystemResourceEnabled *bool   `yaml:"system_resource_enabled"`
	AlertEnabled         *bool   `yaml:"alert_enabled"`
	AlertSuppressEnabled *bool   `yaml:"alert_suppress_enabled"`
	AlertRulesFile       *string `yaml:"alert_rules_file"`
	AlertLogPaths        *string `yaml:"alert_log_paths"`
	AlertPollInterval    *string `yaml:"alert_poll_interval"`
	AlertStartFromEnd    *bool   `yaml:"alert_start_from_end"`
}

func runtimeConfigPath(configPath string) string {
	cleaned := strings.TrimSpace(configPath)
	if cleaned == "" {
		return ""
	}
	ext := filepath.Ext(cleaned)
	if ext == "" {
		return cleaned + ".runtime.yaml"
	}
	return strings.TrimSuffix(cleaned, ext) + ".runtime" + ext
}

func loadRuntimeConfig(configPath string) (*runtimeConfig, error) {
	path := runtimeConfigPath(configPath)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取运行时配置文件失败: %s: %w", path, err)
	}
	var cfg runtimeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析运行时配置文件失败: %s: %w", path, err)
	}
	return &cfg, nil
}

func applyRuntimeConfig(cfg *models.Config, runtime *runtimeConfig) {
	if cfg == nil || runtime == nil {
		return
	}
	if runtime.WatchDir != nil {
		cfg.WatchDir = strings.TrimSpace(*runtime.WatchDir)
	}
	if runtime.FileExt != nil {
		cfg.FileExt = strings.TrimSpace(*runtime.FileExt)
	}
	if runtime.Silence != nil {
		cfg.Silence = strings.TrimSpace(*runtime.Silence)
	}
	if runtime.UploadWorkers != nil {
		cfg.UploadWorkers = *runtime.UploadWorkers
	}
	if runtime.UploadQueueSize != nil {
		cfg.UploadQueueSize = *runtime.UploadQueueSize
	}
	if runtime.SystemResourceEnabled != nil {
		cfg.SystemResourceEnabled = *runtime.SystemResourceEnabled
	}
	if runtime.AlertEnabled != nil {
		cfg.AlertEnabled = *runtime.AlertEnabled
	}
	if runtime.AlertSuppressEnabled != nil {
		cfg.AlertSuppressEnabled = boolPtr(*runtime.AlertSuppressEnabled)
	}
	if runtime.AlertRulesFile != nil {
		cfg.AlertRulesFile = strings.TrimSpace(*runtime.AlertRulesFile)
	}
	if runtime.AlertLogPaths != nil {
		cfg.AlertLogPaths = strings.TrimSpace(*runtime.AlertLogPaths)
	}
	if runtime.AlertPollInterval != nil {
		cfg.AlertPollInterval = strings.TrimSpace(*runtime.AlertPollInterval)
	}
	if runtime.AlertStartFromEnd != nil {
		cfg.AlertStartFromEnd = boolPtr(*runtime.AlertStartFromEnd)
	}
}

func SaveRuntimeConfig(configPath string, cfg *models.Config) error {
	if cfg == nil {
		return nil
	}
	path := runtimeConfigPath(configPath)
	if path == "" {
		return nil
	}
	runtime := buildRuntimeConfig(cfg)
	data, err := yaml.Marshal(runtime)
	if err != nil {
		return fmt.Errorf("序列化运行时配置失败: %w", err)
	}
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("写入运行时配置文件失败: %s: %w", path, err)
	}
	return nil
}

func buildRuntimeConfig(cfg *models.Config) *runtimeConfig {
	if cfg == nil {
		return nil
	}
	suppressEnabled := true
	if cfg.AlertSuppressEnabled != nil {
		suppressEnabled = *cfg.AlertSuppressEnabled
	}
	startFromEnd := true
	if cfg.AlertStartFromEnd != nil {
		startFromEnd = *cfg.AlertStartFromEnd
	}
	pollInterval := strings.TrimSpace(cfg.AlertPollInterval)
	if pollInterval == "" {
		pollInterval = defaultAlertPollInterval
	}
	return &runtimeConfig{
		WatchDir:             stringPtr(strings.TrimSpace(cfg.WatchDir)),
		FileExt:              stringPtr(strings.TrimSpace(cfg.FileExt)),
		Silence:              stringPtr(strings.TrimSpace(cfg.Silence)),
		UploadWorkers:        intPtr(cfg.UploadWorkers),
		UploadQueueSize:      intPtr(cfg.UploadQueueSize),
		SystemResourceEnabled: boolPtr(cfg.SystemResourceEnabled),
		AlertEnabled:         boolPtr(cfg.AlertEnabled),
		AlertSuppressEnabled: boolPtr(suppressEnabled),
		AlertRulesFile:       stringPtr(strings.TrimSpace(cfg.AlertRulesFile)),
		AlertLogPaths:        stringPtr(strings.TrimSpace(cfg.AlertLogPaths)),
		AlertPollInterval:    stringPtr(pollInterval),
		AlertStartFromEnd:    boolPtr(startFromEnd),
	}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, "gwf-config-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}
