// 本文件用于文件监控相关测试
package watcher

import "testing"

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
