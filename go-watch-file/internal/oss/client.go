// 本文件用于 OSS 客户端封装与上传
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package oss

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	sdk "github.com/aliyun/aliyun-oss-go-sdk/oss"

	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
)

// Client 封装 OSS SDK 客户端及相关配置
type Client struct {
	ossClient *sdk.Client
	bucket    *sdk.Bucket
	config    *models.Config
	hostName  string
}

// NewClient 创建并初始化 OSS 客户端
func NewClient(config *models.Config) (*Client, error) {
	logger.Info("初始化OSS客户端...")
	endpoint, err := normalizeOSSEndpoint(config.Endpoint, config.DisableSSL)
	if err != nil {
		return nil, err
	}

	ossClient, err := sdk.New(endpoint, config.AK, config.SK)
	if err != nil {
		return nil, fmt.Errorf("创建OSS客户端失败: %w", err)
	}
	bucket, err := ossClient.Bucket(config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("获取OSS Bucket失败: %w", err)
	}

	logger.Info("OSS客户端初始化成功")
	hostName := normalizeHostName()
	return &Client{
		ossClient: ossClient,
		bucket:    bucket,
		config:    config,
		hostName:  hostName,
	}, nil
}

// UploadFile 上传文件到 OSS 并返回下载链接
func (c *Client) UploadFile(ctx context.Context, filePath string) (string, error) {
	logger.Info("开始上传文件到OSS: %s", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}
	// 固定上传大小，避免文件增长导致上传内容不一致
	contentLength := fileInfo.Size()

	objectKey, err := c.buildObjectKey(filePath)
	if err != nil {
		return "", fmt.Errorf("构建对象Key失败: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c.bucket == nil {
		return "", fmt.Errorf("OSS Bucket未初始化")
	}
	verifyETag := isUploadETagVerifyEnabled(c.config)
	body := io.NewSectionReader(file, 0, contentLength)
	uploadReader := io.Reader(body)
	var hasher hash.Hash
	if verifyETag {
		hasher = md5.New()
		uploadReader = io.TeeReader(body, hasher)
	}
	reader := &contextReader{
		ctx:    ctx,
		reader: uploadReader,
	}
	var responseHeader http.Header
	putOptions := []sdk.Option{
		sdk.ContentLength(contentLength),
		sdk.ContentType("application/octet-stream"),
	}
	if verifyETag {
		putOptions = append(putOptions, sdk.GetResponseHeader(&responseHeader))
	}

	err = c.bucket.PutObject(
		objectKey,
		reader,
		putOptions...,
	)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("OSS上传失败: %w", err)
	}
	if verifyETag {
		if hasher == nil {
			return "", fmt.Errorf("ETag校验器未初始化")
		}
		localMD5 := hex.EncodeToString(hasher.Sum(nil))
		remoteETag := normalizeETag(responseHeader.Get("ETag"))
		if remoteETag == "" {
			return "", fmt.Errorf("OSS上传成功但未返回ETag，无法完成校验: %s", objectKey)
		}
		if !isValidMD5Hex(remoteETag) {
			return "", fmt.Errorf("OSS返回的ETag格式无效，无法与本地MD5比对: etag=%s", remoteETag)
		}
		if !isETagMatch(localMD5, remoteETag) {
			return "", fmt.Errorf("OSS ETag校验失败: local=%s remote=%s", localMD5, remoteETag)
		}
		logger.Info("OSS ETag校验通过: object=%s etag=%s", objectKey, remoteETag)
	}
	logger.Info("OSS上传成功")
	logger.Info("文件同步完成: %s", objectKey)

	downloadURL := c.buildDownloadURL(objectKey)
	logger.Info("下载链接: %s", downloadURL)
	return downloadURL, nil
}

func isUploadETagVerifyEnabled(cfg *models.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.UploadETagVerifyEnabled
}

func normalizeETag(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, "\"")
	return strings.ToLower(trimmed)
}

func isValidMD5Hex(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, ch := range value {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		default:
			return false
		}
	}
	return true
}

func isETagMatch(localMD5Hex, remoteETag string) bool {
	local := normalizeETag(localMD5Hex)
	remote := normalizeETag(remoteETag)
	if !isValidMD5Hex(local) || !isValidMD5Hex(remote) {
		return false
	}
	return local == remote
}

// buildObjectKey 用于构建后续流程所需的数据
func (c *Client) buildObjectKey(filePath string) (string, error) {
	watchDirs := pathutil.SplitWatchDirs(c.config.WatchDir)
	objectKey, err := pathutil.BuildObjectKeyStrictForDirs(watchDirs, filePath)
	if err != nil {
		return "", err
	}
	hostName := strings.TrimSpace(c.hostName)
	if hostName == "" {
		hostName = "unknown-host"
	}
	hostName = strings.Trim(hostName, "/")
	if hostName == "" {
		hostName = "unknown-host"
	}
	if objectKey == "" {
		return hostName, nil
	}
	return hostName + "/" + objectKey, nil
}

// buildDownloadURL 用于构建后续流程所需的数据
func (c *Client) buildDownloadURL(objectKey string) string {
	return pathutil.BuildDownloadURL(
		c.config.Endpoint,
		c.config.Bucket,
		objectKey,
		c.config.ForcePathStyle,
		c.config.DisableSSL,
	)
}

// normalizeOSSEndpoint 用于统一 OSS Endpoint 格式
func normalizeOSSEndpoint(endpoint string, disableSSL bool) (string, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "", fmt.Errorf("OSS Endpoint不能为空")
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return trimmed, nil
	}
	parsed, err = url.Parse("//" + trimmed)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("无效的 OSS Endpoint: %s", endpoint)
	}
	scheme := "https"
	if disableSSL {
		scheme = "http"
	}
	return scheme + "://" + parsed.Host + strings.TrimSuffix(parsed.Path, "/"), nil
}

// contextReader 用于让上传过程响应上下文取消
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Read 在读取前检查上下文，避免取消后继续上传
func (r *contextReader) Read(p []byte) (int, error) {
	if r == nil {
		return 0, io.EOF
	}
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	if r.reader == nil {
		return 0, io.EOF
	}
	n, err := r.reader.Read(p)
	if err != nil {
		return n, err
	}
	if r.ctx != nil {
		if ctxErr := r.ctx.Err(); ctxErr != nil {
			return n, ctxErr
		}
	}
	return n, nil
}

// normalizeHostName 用于统一数据格式便于比较与存储
func normalizeHostName() string {
	host, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	host = strings.TrimSpace(host)
	host = strings.ReplaceAll(host, "/", "-")
	host = strings.ReplaceAll(host, "\\", "-")
	if host == "" {
		return "unknown-host"
	}
	return host
}

// GetClient 返回底层的 OSS SDK 客户端
func (c *Client) GetClient() *sdk.Client {
	return c.ossClient
}
