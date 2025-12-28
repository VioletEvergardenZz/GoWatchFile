package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"file-watch/internal/logger"
)

// Robot 钉钉机器人。
type Robot struct {
	webhook string
	secret  string
}

type message struct {
	MsgType  string   `json:"msgtype"`
	Markdown markdown `json:"markdown"`
}

type markdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type response struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// NewRobot 创建钉钉机器人实例。
func NewRobot(webhook, secret string) *Robot {
	return &Robot{
		webhook: strings.TrimSpace(webhook),
		secret:  strings.TrimSpace(secret),
	}
}

// SendMessage 发送钉钉机器人消息。
func (r *Robot) SendMessage(ctx context.Context, downloadURL, fileName string) error {
	if r.webhook == "" {
		return fmt.Errorf("钉钉 webhook 为空")
	}

	fileName = defaultValue(fileName, "unknown")

	msg := buildMarkdownMessage(downloadURL, fileName)

	jsonReq, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化钉钉消息失败: %w", err)
	}

	webhookURL, err := r.buildWebhookURL()
	if err != nil {
		return fmt.Errorf("构建钉钉 webhook URL 失败: %w", err)
	}

	if err := r.postMessage(ctx, webhookURL, jsonReq); err != nil {
		return err
	}

	logger.Info("钉钉机器人消息发送成功")
	return nil
}

func buildMarkdownMessage(downloadURL, fileName string) message {
	nowTime := time.Now().Format("2006-01-02 15:04:05")
	text := fmt.Sprintf(
		"### File uploaded\n\n- file: `%s`\n- download: [download link](%s)\n- time: %s",
		fileName,
		downloadURL,
		nowTime,
	)
	return message{
		MsgType: "markdown",
		Markdown: markdown{
			Title: "File uploaded",
			Text:  text,
		},
	}
}

func (r *Robot) postMessage(ctx context.Context, webhookURL string, payload []byte) error {
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if ctx != nil {
		req = req.WithContext(ctx)
	}

	logger.Info("开始发送钉钉机器人消息")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送 HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("钉钉机器人 HTTP 状态码异常: %d", resp.StatusCode)
	}

	var responseBody response
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return fmt.Errorf("解析钉钉响应失败: %w", err)
	}
	if responseBody.ErrCode != 0 {
		return fmt.Errorf("钉钉机器人返回错误: %d %s", responseBody.ErrCode, responseBody.ErrMsg)
	}
	return nil
}

// 如果配置了 secret，钉钉要求在请求时把 timestamp 和 sign 作为 query 参数拼上去，
// 所以 buildWebhookURL() 会基于配置里的 webhook 解析并追加这两个参数再返回
func (r *Robot) buildWebhookURL() (string, error) {
	if r.secret == "" {
		return r.webhook, nil
	}

	timestamp := time.Now().UnixMilli()
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, r.secret)

	mac := hmac.New(sha256.New, []byte(r.secret))
	_, _ = mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	parsedURL, err := url.Parse(r.webhook)
	if err != nil {
		return "", err
	}

	query := parsedURL.Query()
	query.Set("timestamp", fmt.Sprintf("%d", timestamp))
	query.Set("sign", sign)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
