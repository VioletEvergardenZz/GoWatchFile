package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// Robot 企业微信机器人
type Robot struct {
	robotKey string
}

// NewRobot 创建新的企业微信机器人
func NewRobot(robotKey string) *Robot {
	return &Robot{
		robotKey: robotKey,
	}
}

// SendMessage 发送消息到企业微信机器人
/**
参数：
downloadUrl：S3 上传后得到的文件下载链接（用于在消息中提供下载）
filepath：本地/对象路径字符串，代码里用它来解析 app 名
返回值：
若发送成功返回 nil，否则返回带有错误信息的 error。
*/
func (r *Robot) SendMessage(downloadUrl, filepath string) error {
	logger.Info("开始发送企业微信机器人消息")
	//构建 webhook URL
	url := fmt.Sprintf("%s%s", "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=", r.robotKey)
	method := "POST"

	app := strings.Split(filepath, "/")[4]
	nowTime := time.Now().Format("2006-01-02 15:04:05")

	markdownMessage := fmt.Sprintf("### app: <font color=\"warning\">%s</font> \r\n ### 下载路径: [%s](%s) \r\n ### datetime: <font color=\"info\">%s</font>", app, downloadUrl, downloadUrl, nowTime)

	msg := models.WxRotMessage{
		MarkDown: models.Markdown{
			Content: markdownMessage,
		},
		MsgType: "markdown",
	}

	jsonReq, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	//使用 http.NewRequest 构建 POST 请求，body 是 JSON bytes。
	//设置 Content-Type 为 application/json，企业微信 webhook 要求如此。
	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonReq))
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		logger.Info("企业微信机器人消息发送成功")
		return nil
	} else {
		return fmt.Errorf("企业微信机器人消息发送失败，状态码: %d", resp.StatusCode)
	}
}
