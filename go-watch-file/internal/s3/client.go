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
type Client struct {
	s3Client *s3.S3
	config   *models.Config
}

// NewClient 创建新的S3客户端
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

	// 创建会话
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("创建S3会话失败: %v", err)
	}

	// 创建S3客户端
	s3Client := s3.New(sess)

	logger.Info("S3客户端初始化成功")
	return &Client{
		s3Client: s3Client,
		config:   config,
	}, nil
}

// UploadFile 上传文件到S3
func (c *Client) UploadFile(filePath string) (string, error) {
	logger.Info("开始上传文件到S3: %s", filePath)

	// 打开文件
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

	// 构建S3对象键
	objectKey := filePath
	if c.config.ForcePathStyle {
		// 如果使用路径样式，保持完整路径
		objectKey = filePath
	}

	// 创建上传输入
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

// GetClient 获取S3客户端实例
func (c *Client) GetClient() *s3.S3 {
	return c.s3Client
}
