package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"file-watch/internal/models"
)

const (
	defaultUploadWorkers   = 3
	defaultUploadQueueSize = 100
	defaultLogLevel        = "info"
)

// LoadConfig 加载配置文件并应用默认值。
func LoadConfig(configFile string) (*models.Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg models.Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

// ValidateConfig 验证配置必填项。
func ValidateConfig(config *models.Config) error {
	if err := requireValue(config.WatchDir, "监控目录"); err != nil {
		return err
	}
	if err := requireValue(config.FileExt, "文件后缀"); err != nil {
		return err
	}
	if err := requireValue(config.Bucket, "S3 Bucket"); err != nil {
		return err
	}
	if config.AK == "" || config.SK == "" {
		return fmt.Errorf("S3认证信息不能为空")
	}
	if err := requireValue(config.Endpoint, "S3 Endpoint"); err != nil {
		return err
	}
	if err := requireValue(config.Region, "S3 Region"); err != nil {
		return err
	}
	if err := requireValue(config.JenkinsHost, "Jenkins Host"); err != nil {
		return err
	}
	if err := requireValue(config.JenkinsJob, "Jenkins Job"); err != nil {
		return err
	}

	return nil
}

func applyDefaults(cfg *models.Config) {
	if cfg.UploadWorkers <= 0 {
		cfg.UploadWorkers = defaultUploadWorkers
	}
	if cfg.UploadQueueSize <= 0 {
		cfg.UploadQueueSize = defaultUploadQueueSize
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
}

func requireValue(value, name string) error {
	if value == "" {
		return fmt.Errorf("%s不能为空", name)
	}
	return nil
}
