package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"file-watch/internal/logger"
)

const (
	webhookURLFormat = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s"
	timeFormat       = "2006-01-02 15:04:05"
	messageTemplate  = "### app: <font color=\"warning\">%s</font> \r\n ### 下载路径: [%s](%s) \r\n ### datetime: <font color=\"info\">%s</font>"
)

// Robot 企业微信机器人
type Robot struct {
	robotKey string
}

type message struct {
	MsgType  string   `json:"msgtype"`
	Markdown markdown `json:"markdown"`
}

type markdown struct {
	Content string `json:"content"`
}

// NewRobot 创建新的企业微信机器人。
func NewRobot(robotKey string) *Robot {
	return &Robot{
		robotKey: robotKey,
	}
}

// SendMessage 发送消息到企业微信机器人，appName 为监控目录下的应用名。
func (r *Robot) SendMessage(downloadURL, appName string) error {
	logger.Info("开始发送企业微信机器人消息")
	if appName == "" {
		return fmt.Errorf("应用名称不能为空")
	}

	msg := buildMarkdownMessage(appName, downloadURL, time.Now())

	jsonReq, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	if err := sendRequest(buildWebhookURL(r.robotKey), jsonReq); err != nil {
		return err
	}

	logger.Info("企业微信机器人消息发送成功")
	return nil
}

func buildWebhookURL(robotKey string) string {
	return fmt.Sprintf(webhookURLFormat, robotKey)
}

func buildMarkdownMessage(appName, downloadURL string, now time.Time) message {
	nowTime := now.Format(timeFormat)
	markdownMessage := fmt.Sprintf(messageTemplate, appName, downloadURL, downloadURL, nowTime)
	return message{
		Markdown: markdown{
			Content: markdownMessage,
		},
		MsgType: "markdown",
	}
}

func sendRequest(url string, payload []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信机器人消息发送失败，状态码: %d", resp.StatusCode)
	}
	return nil
}
