// 本文件用于邮件通知发送
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second

// Sender 负责发送 SMTP 邮件
type Sender struct {
	host     string
	port     int
	user     string
	password string
	from     string
	to       []string
	useTLS   bool
}

// NewSender 创建邮件发送器
func NewSender(host string, port int, user, password, from string, to []string, useTLS bool) *Sender {
	return &Sender{
		host:     strings.TrimSpace(host),
		port:     port,
		user:     strings.TrimSpace(user),
		password: password,
		from:     strings.TrimSpace(from),
		to:       cleanRecipients(to), //to 先清理空元素和多余空格
		useTLS:   useTLS,              //useTLS 控制 TLS 与 STARTTLS 行为
	}
}

// SendMessage 通过 SMTP 发送邮件
func (s *Sender) SendMessage(ctx context.Context, subject, body string) error {
	// 统一做入参校验与默认超时，避免连接长时间挂起
	// sender 本身不能为空
	if s == nil {
		return fmt.Errorf("email sender is nil")
	}
	// SMTP 主机必填
	if s.host == "" {
		return fmt.Errorf("smtp host is empty")
	}
	// 端口必须为正数
	if s.port <= 0 {
		return fmt.Errorf("smtp port is invalid")
	}
	// 发件人不能为空
	if s.from == "" {
		return fmt.Errorf("smtp from is empty")
	}
	// 收件人不能为空
	if len(s.to) == 0 {
		return fmt.Errorf("smtp recipients are empty")
	}

	// 未传入上下文时使用背景上下文
	if ctx == nil {
		ctx = context.Background()
	}
	// 没有超时时间就设置默认超时
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	// 先建立 TCP 连接，再根据端口与配置决定是否走 TLS 或 STARTTLS
	// 拼接 host 与 port
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	// dialer 负责建立 TCP 连接
	dialer := net.Dialer{Timeout: defaultTimeout}
	// 允许 ctx 取消连接过程
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		// TCP 连接失败直接返回
		return fmt.Errorf("smtp dial failed: %w", err)
	}
	// 将 ctx 的截止时间同步到连接读写
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// client 表示 SMTP 会话
	var client *smtp.Client
	if s.useTLS && s.port == 465 {
		// 465 端口通常走 SMTPS 直连 TLS
		tlsConn := tls.Client(conn, &tls.Config{ServerName: s.host})
		// TLS 握手失败需要关闭连接
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()
			return fmt.Errorf("smtp tls handshake failed: %w", err)
		}
		// TLS 握手成功后创建 SMTP 客户端
		client, err = smtp.NewClient(tlsConn, s.host)
	} else {
		// 其他端口先建立明文 SMTP 会话
		client, err = smtp.NewClient(conn, s.host)
	}
	if err != nil {
		// SMTP 客户端创建失败需要关闭连接
		_ = conn.Close()
		return fmt.Errorf("smtp client init failed: %w", err)
	}
	// 确保会话结束时关闭客户端
	defer client.Close()

	// 仅在非 465 端口时尝试 STARTTLS
	if s.useTLS && s.port != 465 {
		// 检查服务器是否支持 STARTTLS
		if ok, _ := client.Extension("STARTTLS"); ok {
			// 将连接升级为 TLS
			if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
				return fmt.Errorf("smtp starttls failed: %w", err)
			}
		} else {
			// 服务器不支持 STARTTLS 直接失败
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
	}

	// 若配置了账号，则进行 AUTH 认证
	if s.user != "" {
		// 检查服务器是否支持 AUTH
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp server does not support AUTH")
		}
		// 使用 PLAIN 认证方式
		auth := smtp.PlainAuth("", s.user, s.password, s.host)
		// 认证失败直接返回
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	// MAIL FROM 设置信封发件人
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("smtp mail from failed: %w", err)
	}
	// RCPT TO 逐个设置收件人
	for _, rcpt := range s.to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt to %s failed: %w", rcpt, err)
		}
	}
	// Data 阶段写入完整邮件内容
	// DATA 开始写入邮件内容
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	// 写入邮件头与正文
	if _, err := writer.Write([]byte(buildMessage(s.from, s.to, subject, body))); err != nil {
		// 写入失败需关闭 writer
		_ = writer.Close()
		return fmt.Errorf("smtp write failed: %w", err)
	}
	// DATA 结束并提交邮件
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp data close failed: %w", err)
	}
	// QUIT 请求用于优雅关闭会话
	if err := client.Quit(); err != nil {
		// QUIT 失败不影响邮件已发送成功的事实
		return &QuitError{Err: err}
	}
	return nil
}

// QuitError 表示邮件发送完成后 SMTP QUIT 失败
type QuitError struct {
	// Err 保存底层 QUIT 错误
	Err error
}

// Error 返回可读的 QUIT 失败描述
func (e *QuitError) Error() string {
	// 保持对外统一的错误文本格式
	if e == nil || e.Err == nil {
		return "smtp quit failed"
	}
	return fmt.Sprintf("smtp quit failed: %v", e.Err)
}

// Unwrap 暴露底层错误，方便调用方用 errors.As 判断
func (e *QuitError) Unwrap() error {
	// 支持 errors.As 解包
	if e == nil {
		return nil
	}
	return e.Err
}

// IsQuitError 判断错误是否为退出失败
func IsQuitError(err error) bool {
	// 用于外部判断是否属于 QUIT 异常
	var quitErr *QuitError
	return errors.As(err, &quitErr)
}

// buildMessage 组装标准 SMTP 文本邮件内容
// 这里不引入复杂 MIME，保证告警通知稳定可用
func buildMessage(from string, to []string, subject, body string) string {
	// 这里使用简单文本邮件头，避免 MIME 复杂性
	// Subject 去除换行避免头注入
	cleanSubject := strings.NewReplacer("\r", "", "\n", "").Replace(subject)
	// 基础邮件头
	headers := []string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", strings.Join(to, ", ")),
		fmt.Sprintf("Subject: %s", cleanSubject),
		fmt.Sprintf("Date: %s", time.Now().Format(time.RFC1123Z)),
		// 统一为 UTF-8 文本
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"UTF-8\"",
	}
	// 正文换行转换为 CRLF
	normalizedBody := normalizeLineEndings(body)
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + normalizedBody + "\r\n"
}

// normalizeLineEndings 统一换行符为 CRLF，满足 SMTP 协议要求
func normalizeLineEndings(body string) string {
	// SMTP 要求 CRLF，统一转换换行符格式
	// 先收敛为 LF 再转为 CRLF
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.ReplaceAll(body, "\n", "\r\n")
}

// cleanRecipients 清理收件人列表中的空项与多余空格
func cleanRecipients(list []string) []string {
	// 清理收件人列表中的空值与多余空格
	// 保持收件人顺序以便对照发送结果
	out := make([]string, 0, len(list))
	for _, item := range list {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
