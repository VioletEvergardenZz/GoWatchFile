// Package s3
/**
创建一个封装了 SDK 的 Client，并提供上传方法
*/
package s3

import (
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// Client S3客户端
/**
保存 S3 SDK 的客户端和配置信息，便于把它作为依赖注入到上传 worker 或其他模块。
*/
type Client struct {
	s3Client *s3.S3
	config   *models.Config
}

// NewClient
/**
目的：创建并初始化一个 Client 实例。
参数：*models.Config（指针，避免复制大型结构并直接读配置）。
返回：(*Client, error)，成功返回客户端指针，失败返回错误。
*/
func NewClient(config *models.Config) (*Client, error) {
	logger.Info("初始化S3客户端...")
	// 创建AWS配置
	awsConfig := &aws.Config{
		Region:           aws.String(config.Region),
		Credentials:      credentials.NewStaticCredentials(config.AK, config.SK, ""),
		Endpoint:         aws.String(config.Endpoint),
		DisableSSL:       aws.Bool(config.DisableSSL),
		S3ForcePathStyle: aws.Bool(config.ForcePathStyle),
	}
	// 创建 AWS 会话（session），若创建失败返回错误；
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("创建S3会话失败: %v", err)
	}
	// 构建 S3 客户端实例
	s3Client := s3.New(sess)
	logger.Info("S3客户端初始化成功")
	return &Client{
		s3Client: s3Client,
		config:   config,
	}, nil
}

// UploadFile
/**
目的：把本地文件上传到配置的 S3 bucket，返回下载 URL 或错误。
参数：filePath string — 本地文件路径。
返回：(string, error) — 成功返回构造的下载 URL，失败返回错误。
*/
func (c *Client) UploadFile(filePath string) (string, error) {
	logger.Info("开始上传文件到S3: %s", filePath)
	// 打开本地文件并在函数结束时关闭。若打开失败（文件不存在或权限问题），返回错误
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %v", err)
	}

	// 构建S3对象键，把本地路径直接当作 S3 key
	//TODO 没理解这个
	objectKey := filePath
	if c.config.ForcePathStyle {
		// 如果使用路径样式，保持完整路径
		objectKey = filePath
	}

	// 构建上传请求
	input := &s3.PutObjectInput{
		Bucket:        aws.String(c.config.Bucket),
		Key:           aws.String(objectKey),
		Body:          file,
		ContentLength: aws.Int64(fileInfo.Size()),
		ContentType:   aws.String("application/octet-stream"),
	}

	// 执行上传
	output, err := c.s3Client.PutObject(input)
	if err != nil {
		return "", fmt.Errorf("S3上传失败: %v", err)
	}
	logger.Info("S3上传成功 - ETag: %s", *output.ETag)
	logger.Info("文件同步完成: %s", objectKey)

	// 构建下载链接
	var downloadUrl string
	if c.config.ForcePathStyle {
		// 路径样式：http://endpoint/bucket/key
		downloadUrl = fmt.Sprintf("http://%s/%s%s", c.config.Endpoint, c.config.Bucket, objectKey)
	} else {
		// 虚拟主机样式：http://bucket.endpoint/key
		downloadUrl = fmt.Sprintf("http://%s.%s%s", c.config.Bucket, c.config.Endpoint, objectKey)
	}

	// 如果启用了HTTPS，使用https
	if !c.config.DisableSSL {
		downloadUrl = strings.Replace(downloadUrl, "http://", "https://", 1)
	}
	logger.Info("下载链接: %s", downloadUrl)
	return downloadUrl, nil
}

// GetClient
/**
简单返回内部 s3Client，供其他包需要直接访问 AWS SDK 时使用。
*/
func (c *Client) GetClient() *s3.S3 {
	return c.s3Client
}
