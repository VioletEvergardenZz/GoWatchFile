package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"file-watch/internal/models"
)

// LoadConfig 加载配置文件
func LoadConfig(configFile string) (*models.Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config models.Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	if config.UploadWorkers <= 0 {
		config.UploadWorkers = 3
	}
	if config.UploadQueueSize <= 0 {
		config.UploadQueueSize = 100
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	return &config, nil
}

// ValidateConfig 验证配置
func ValidateConfig(config *models.Config) error {
	if config.WatchDir == "" {
		return fmt.Errorf("监控目录不能为空")
	}
	if config.FileExt == "" {
		return fmt.Errorf("文件后缀不能为空")
	}
	if config.Bucket == "" {
		return fmt.Errorf("S3 Bucket不能为空")
	}
	if config.AK == "" || config.SK == "" {
		return fmt.Errorf("S3认证信息不能为空")
	}
	if config.Endpoint == "" {
		return fmt.Errorf("S3 Endpoint不能为空")
	}
	if config.Region == "" {
		return fmt.Errorf("S3 Region不能为空")
	}
	if config.JenkinsHost == "" {
		return fmt.Errorf("Jenkins Host不能为空")
	}
	if config.JenkinsJob == "" {
		return fmt.Errorf("Jenkins Job不能为空")
	}

	return nil
}
