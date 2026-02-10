// 本文件用于文件监控相关测试
package watcher

import (
	"testing"
	"time"
)

func TestIsTempFile(t *testing.T) {
	cases := []struct {
		name     string
		filePath string
		want     bool
	}{
		{name: "tmp", filePath: "/tmp/a.tmp", want: true},
		{name: "part", filePath: "a.part", want: true},
		{name: "crdownload", filePath: "a.crdownload", want: true},
		{name: "download", filePath: "a.download", want: true},
		{name: "swp", filePath: "a.swp", want: true},
		{name: "swx", filePath: "a.swx", want: true},
		{name: "swpx", filePath: "a.swpx", want: true},
		{name: "uppercase", filePath: "A.TMP", want: true},
		{name: "nested", filePath: "/a/b/c.tmp", want: true},
		{name: "no-suffix", filePath: "a", want: false},
		{name: "similar", filePath: "a.tmpx", want: false},
		{name: "empty", filePath: "", want: false},
		{name: "dot", filePath: ".", want: false},
		{name: "root", filePath: "/", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isTempFile(tc.filePath)
			if got != tc.want {
				t.Fatalf("isTempFile(%q) = %v, want %v", tc.filePath, got, tc.want)
			}
		})
	}
}

func TestParseSilenceWindow(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "duration string", raw: "15s", want: 15 * time.Second},
		{name: "chinese seconds", raw: "20秒", want: 20 * time.Second},
		{name: "plain number", raw: "30", want: 30 * time.Second},
		{name: "invalid fallback", raw: "bad", want: defaultSilenceWindow},
		{name: "empty fallback", raw: "", want: defaultSilenceWindow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSilenceWindow(tc.raw)
			if got != tc.want {
				t.Fatalf("parseSilenceWindow(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
