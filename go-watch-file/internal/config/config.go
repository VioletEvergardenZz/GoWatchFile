package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"file-watch/internal/models"
)

// LoadConfig 加载配置文件
// 参数：configFile string — 配置文件路径（可以是相对路径或绝对路径），通常来自命令行 -config 参数
// 返回值：(*models.Config, error) — 成功时返回指向 models.Config 的指针和 nil 错误；失败时返回 nil 和非空 error（包含失败原因）。
func LoadConfig(configFile string) (*models.Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config models.Config
	//使用 YAML 解码器把字节流解析到 models.Config（字段通过 yaml:"..." 标签映射）
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	//如果某些值在 YAML 中没有设置，结构体字段保留 Go 的“零值”，代码在这里把一些零值换成合理默认值（上传 worker、队列大小、日志级别）。
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
// 参数：config *models.Config — LoadConfig 解出的配置结构体指针
// 返回值：error — 如果配置合法返回 nil；否则返回一个描述问题的 error（例如某些必填项为空）
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
