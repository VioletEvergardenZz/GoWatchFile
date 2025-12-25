package pathutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelativePath_PreventsSymlinkEscape(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()

	escapeLink := filepath.Join(baseDir, "escape")
	if err := os.Symlink(outsideDir, escapeLink); err != nil {
		skipIfSymlinkNotSupported(t, err)
		return
	}

	outsideFile := filepath.Join(outsideDir, "out.txt")
	if err := os.WriteFile(outsideFile, []byte("data"), 0o644); err != nil {
			t.Fatalf("写入外部文件失败: %v", err)
	}

	_, err := RelativePath(baseDir, filepath.Join(escapeLink, "out.txt"))
	if err == nil || !errors.Is(err, ErrOutsideBaseDir) {
		t.Fatalf("期望返回外部目录错误，实际: %v", err)
	}
}

func TestRelativePath_AllowsSymlinkInside(t *testing.T) {
	baseDir := t.TempDir()
	realDir := filepath.Join(baseDir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	linkDir := filepath.Join(baseDir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		skipIfSymlinkNotSupported(t, err)
		return
	}

	targetFile := filepath.Join(realDir, "file.txt")
	if err := os.WriteFile(targetFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	rel, err := RelativePath(baseDir, filepath.Join(linkDir, "file.txt"))
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if rel != "real/file.txt" {
		t.Fatalf("相对路径不符合预期，实际 %q", rel)
	}
}

func TestBuildObjectKey_StrictAndPermissive(t *testing.T) {
	baseDir := t.TempDir()
	fileInDir := filepath.Join(baseDir, "a.txt")
	if err := os.WriteFile(fileInDir, []byte("data"), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	key, err := BuildObjectKeyStrict(baseDir, fileInDir)
	if err != nil {
		t.Fatalf("严格模式构建失败: %v", err)
	}
	expectedStrict := trimLeadingSlash(joinURLPath(trimLeadingSlash(toSlashPath(baseDir)), "a.txt"))
	if key != expectedStrict {
		t.Fatalf("严格模式生成的 key 不符合预期: %q（期望 %q）", key, expectedStrict)
	}

	outsideFile := filepath.Join(t.TempDir(), "b.txt")
	if err := os.WriteFile(outsideFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("写入外部文件失败: %v", err)
	}
	if _, err := BuildObjectKeyStrict(baseDir, outsideFile); !errors.Is(err, ErrOutsideBaseDir) {
		t.Fatalf("期望返回外部目录错误，实际: %v", err)
	}
	permKey := BuildObjectKeyPermissive(baseDir, outsideFile)
	expected := trimLeadingSlash(toSlashPath(outsideFile))
	if permKey != expected {
		t.Fatalf("宽松模式生成的 key 不符合预期: %q（期望 %q）", permKey, expected)
	}
}

func TestBuildDownloadURL_PathStyleAndEscape(t *testing.T) {
	u := BuildDownloadURL("https://example.com:9000/api", "my-bucket", "folder/a b#c?.txt", true, false)
	want := "https://example.com:9000/api/my-bucket/folder/a%20b%23c%3F.txt"
	if u != want {
		t.Fatalf("下载 URL 不符合预期:\n  实际: %s\n  期望: %s", u, want)
	}
}

func TestBuildDownloadURL_VirtualHostEndpointWithoutScheme(t *testing.T) {
	u := BuildDownloadURL("example.com:9000/base", "bucket", "a b.txt", false, true)
	want := "http://bucket.example.com:9000/base/a%20b.txt"
	if u != want {
		t.Fatalf("下载 URL 不符合预期:\n  实际: %s\n  期望: %s", u, want)
	}
}

func skipIfSymlinkNotSupported(t *testing.T, err error) {
	t.Helper()
	if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "privilege") {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
}
