// Package s3 提供对 S3 的简单封装。
package s3

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
)

// Client 封装 S3 SDK 客户端及相关配置。
type Client struct {
	s3Client *s3.S3
	config   *models.Config
}

// NewClient 创建并初始化 S3 客户端。
func NewClient(config *models.Config) (*Client, error) {
	logger.Info("初始化S3客户端...")
	awsConfig := &aws.Config{
		Region:           aws.String(config.Region),
		Credentials:      credentials.NewStaticCredentials(config.AK, config.SK, ""),
		Endpoint:         aws.String(config.Endpoint),
		DisableSSL:       aws.Bool(config.DisableSSL),
		S3ForcePathStyle: aws.Bool(config.ForcePathStyle),
	}
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("创建S3会话失败: %w", err)
	}
	s3Client := s3.New(sess)
	logger.Info("S3客户端初始化成功")
	return &Client{
		s3Client: s3Client,
		config:   config,
	}, nil
}

// UploadFile 上传文件到 S3 并返回下载链接。
func (c *Client) UploadFile(ctx context.Context, filePath string) (string, error) {
	logger.Info("开始上传文件到S3: %s", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}
	// 固定上传大小，避免文件增长导致 ContentLength 与实际 Body 长度不一致
	contentLength := fileInfo.Size()

	objectKey, err := c.buildObjectKey(filePath)
	if err != nil {
		return "", fmt.Errorf("构建对象Key失败: %w", err)
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(c.config.Bucket),
		Key:    aws.String(objectKey),
		// 只读取固定长度，保证与 ContentLength 匹配
		Body:          io.NewSectionReader(file, 0, contentLength),
		ContentLength: aws.Int64(contentLength),
		ContentType:   aws.String("application/octet-stream"),
	}

	if ctx == nil {
		ctx = context.Background()
	}

	output, err := c.s3Client.PutObjectWithContext(ctx, input)
	if err != nil {
		return "", fmt.Errorf("S3上传失败: %w", err)
	}
	if output.ETag != nil {
		logger.Info("S3上传成功 - ETag: %s", *output.ETag)
	} else {
		logger.Info("S3上传成功")
	}
	logger.Info("文件同步完成: %s", objectKey)

	downloadURL := c.buildDownloadURL(objectKey)
	logger.Info("下载链接: %s", downloadURL)
	return downloadURL, nil
}

func (c *Client) buildObjectKey(filePath string) (string, error) {
	return pathutil.BuildObjectKeyStrict(c.config.WatchDir, filePath)
}

func (c *Client) buildDownloadURL(objectKey string) string {
	return pathutil.BuildDownloadURL(
		c.config.Endpoint,
		c.config.Bucket,
		objectKey,
		c.config.ForcePathStyle,
		c.config.DisableSSL,
	)
}

// GetClient 返回底层的 S3 SDK 客户端。
func (c *Client) GetClient() *s3.S3 {
	return c.s3Client
}
