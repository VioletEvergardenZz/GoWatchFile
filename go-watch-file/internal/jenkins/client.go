package jenkins

import (
	"context"
	"fmt"
	"strings"

	"github.com/bndr/gojenkins"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// Client Jenkins客户端
type Client struct {
	jenkins *gojenkins.Jenkins
	config  *models.Config
}

// NewClient 创建新的Jenkins客户端
func NewClient(config *models.Config) (*Client, error) {
	logger.Info("初始化Jenkins客户端...")

	jenkins := gojenkins.CreateJenkins(nil, config.JenkinsHost, config.JenkinsUser, config.JenkinsPassword)
	_, err := jenkins.Init(context.Background())
	if err != nil {
		return nil, fmt.Errorf("jenkins初始化失败: %v", err)
	}

	logger.Info("Jenkins连接成功: %s", config.JenkinsHost)
	return &Client{
		jenkins: jenkins,
		config:  config,
	}, nil
}

// BuildJob 触发Jenkins构建任务
func (c *Client) BuildJob(downloadFile, appName, fileName string) error {
	logger.Info("开始触发Jenkins构建任务 - 应用: %s, 文件名: %s", appName, fileName)

	ctx := context.Background()

	buildParameter := make(map[string]string)
	buildParameter["DOWNLOAD_FILE"] = downloadFile
	buildParameter["APP"] = appName
	buildParameter["FILE_NAME"] = fileName

	logger.Info("Jenkins构建参数: %+v", buildParameter)

	_, err := c.jenkins.BuildJob(ctx, c.config.JenkinsJob, buildParameter)
	if err != nil {
		logger.Error("Jenkins构建任务触发失败: %v", err)
		return err
	}

	logger.Info("Jenkins构建任务触发成功")
	return nil
}

// GetJenkins 获取Jenkins实例
func (c *Client) GetJenkins() *gojenkins.Jenkins {
	return c.jenkins
}

// ParseFileInfo 解析文件信息
func ParseFileInfo(objectKey string) (string, string, error) {
	parts := strings.Split(objectKey, "/")
	if len(parts) < 4 {
		return "", "", fmt.Errorf("文件路径格式不正确: %s", objectKey)
	}

	appName := parts[3]
	fileName := strings.Split(parts[len(parts)-1], ".")[0]

	return appName, fileName, nil
}
