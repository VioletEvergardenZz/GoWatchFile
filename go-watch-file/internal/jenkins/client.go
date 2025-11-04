package jenkins

import (
	"context" //用于传递上下文（例如超时、取消）给 Jenkins SDK 的调用
	"fmt"
	"strings"

	"github.com/bndr/gojenkins"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// Client Jenkins 客户端的结构体定义
/**
字段说明：
jenkins：指向 gojenkins 库中的 Jenkins 实例（客户端），通过它可以调用 Jenkins API（例如 BuildJob、GetJob 等）
config：指向项目配置，包含 Jenkins 的 host、user、password、job 名称等
使用指针的原因：
避免复制，提供 nil 检查，便于共享同一实例在多个 goroutine 中使用
gojenkins.Jenkins 在内部持有网络会话与状态，复用单一实例更高效
*/
type Client struct {
	jenkins *gojenkins.Jenkins
	config  *models.Config
}

// NewClient 创建新的Jenkins客户端
/**
参数：config *models.Config — 配置信息（应包含 JenkinsHost、JenkinsUser、JenkinsPassword）
返回：(*Client, error) — 成功返回 Client 指针，否则返回 error 描述失败原因。
*/
func NewClient(config *models.Config) (*Client, error) {
	logger.Info("初始化Jenkins客户端...")
	//创建客户端实例。第一个参数是 http.Client，传 nil 表示使用默认 HTTP 客户端
	jenkins := gojenkins.CreateJenkins(nil, config.JenkinsHost, config.JenkinsUser, config.JenkinsPassword)
	//初始化连接（内部可能会尝试获取 Jenkins 版本、CSRF token、用户信息等）。传入 context.Background()，没有 timeout/cancellation
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
/**
参数：
downloadFile string：构建参数中传递的下载地址（通常是 S3 上的文件 URL）。
appName string：应用名称（构建参数）。
fileName string：文件名（构建参数）。
返回：error — 触发构建失败时返回错误，否则返回 nil。
*/
func (c *Client) BuildJob(downloadFile, appName, fileName string) error {
	logger.Info("开始触发Jenkins构建任务 - 应用: %s, 文件名: %s", appName, fileName)

	// 创建一个空的context，用于控制请求的生命周期
	ctx := context.Background()
	// 构造传递给Jenkins任务的参数映射，map[string]string 表示键和值都是字符串类型，make() 函数用于创建并初始化这个 map
	buildParameter := make(map[string]string)
	buildParameter["DOWNLOAD_FILE"] = downloadFile
	buildParameter["APP"] = appName
	buildParameter["FILE_NAME"] = fileName
	logger.Info("Jenkins构建参数: %+v", buildParameter)

	//返回构建编号或构建对象的相关信息（这里忽略返回值，只检查 error）
	_, err := c.jenkins.BuildJob(ctx, c.config.JenkinsJob, buildParameter)
	if err != nil {
		logger.Error("Jenkins构建任务触发失败: %v", err)
		return err
	}
	logger.Info("Jenkins构建任务触发成功")
	return nil
}

// GetJenkins 获取Jenkins实例
/**
简单访问器，返回内部的 gojenkins.Jenkins 实例，供其他模块直接调用 SDK 的低级 API（例如获取 job、获取构建状态等）。
注意：调用者在使用返回的指针前应该确保 Client 已初始化成功。
*/
func (c *Client) GetJenkins() *gojenkins.Jenkins {
	return c.jenkins
}

// ParseFileInfo 解析文件信息
/**
从 objectKey（例如 S3 对象键或某个路径）解析出 appName 和 fileName（不带扩展名），按当前实现的规则提取
*/
func ParseFileInfo(objectKey string) (string, string, error) {
	//假设我们有一个 objectKey 值为: "env/dev/myapp/example.mat"
	parts := strings.Split(objectKey, "/")
	if len(parts) < 4 {
		return "", "", fmt.Errorf("文件路径格式不正确: %s", objectKey)
	}
	appName := parts[3]
	// 将文件名按照 "." 分割，取第一部分作为不带扩展名的文件名
	// 例如 "example.mat" 会被分割为 ["example", "mat"]，取索引0的元素 "example"
	fileNameParts := strings.Split(parts[len(parts)-1], ".")
	fileName := fileNameParts[0]
	return appName, fileName, nil
}
