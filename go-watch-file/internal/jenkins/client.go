package jenkins

import (
	"context"
	"fmt"

	"github.com/bndr/gojenkins"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// Client 封装 Jenkins SDK 客户端与配置。
type Client struct {
	jenkins *gojenkins.Jenkins
	config  *models.Config
}

// NewClient 创建新的 Jenkins 客户端。
func NewClient(config *models.Config) (*Client, error) {
	logger.Info("初始化Jenkins客户端...")
	jenkins := gojenkins.CreateJenkins(nil, config.JenkinsHost, config.JenkinsUser, config.JenkinsPassword)
	_, err := jenkins.Init(context.Background())
	if err != nil {
		return nil, fmt.Errorf("jenkins初始化失败: %w", err)
	}

	logger.Info("Jenkins连接成功: %s", config.JenkinsHost)
	return &Client{
		jenkins: jenkins,
		config:  config,
	}, nil
}

// BuildJob 触发 Jenkins 构建任务。
func (c *Client) BuildJob(downloadFile, appName, fileName string) error {
	logger.Info("开始触发Jenkins构建任务 - 应用: %s, 文件名: %s", appName, fileName)

	ctx := context.Background()
	buildParameter := buildParameters(downloadFile, appName, fileName)
	logger.Info("Jenkins构建参数: %+v", buildParameter)

	_, err := c.jenkins.BuildJob(ctx, c.config.JenkinsJob, buildParameter)
	if err != nil {
		logger.Error("Jenkins构建任务触发失败: %v", err)
		return err
	}
	logger.Info("Jenkins构建任务触发成功")
	return nil
}

// GetJenkins 获取 Jenkins SDK 实例。
func (c *Client) GetJenkins() *gojenkins.Jenkins {
	return c.jenkins
}

func buildParameters(downloadFile, appName, fileName string) map[string]string {
	return map[string]string{
		"DOWNLOAD_FILE": downloadFile,
		"APP":           appName,
		"FILE_NAME":     fileName,
	}
}
