package dingtalk

import (
	"strings"
	"testing"
)

func TestBuildMarkdownMessageIncludesAISummary(t *testing.T) {
	msg := buildMarkdownMessage("https://example.com/download.log", "node-a/var/log/app.log", "故障级别=高；根因判断=检测到连续超时；处置建议=检查上游连接池")

	if msg.Markdown.Title != "File uploaded" {
		t.Fatalf("unexpected title: %s", msg.Markdown.Title)
	}
	if !strings.Contains(msg.Markdown.Text, "- ai: 故障级别=高；根因判断=检测到连续超时；处置建议=检查上游连接池") {
		t.Fatalf("ai summary not found in markdown text: %s", msg.Markdown.Text)
	}
}

func TestBuildMarkdownMessageSkipsAILineWhenEmpty(t *testing.T) {
	msg := buildMarkdownMessage("https://example.com/download.log", "node-a/var/log/app.log", "   ")

	if strings.Contains(msg.Markdown.Text, "- ai:") {
		t.Fatalf("ai line should be hidden when summary is empty: %s", msg.Markdown.Text)
	}
}
