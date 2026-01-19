// 本文件用于路径工具的单元测试
package pathutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 覆盖对象 key 与下载链接的构建行为
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

func TestRelativePathAny_PicksDeepestBase(t *testing.T) {
	baseDir := t.TempDir()
	nestedDir := filepath.Join(baseDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	fileInNested := filepath.Join(nestedDir, "deep.txt")
	if err := os.WriteFile(fileInNested, []byte("data"), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	base, rel, err := RelativePathAny([]string{baseDir, nestedDir}, fileInNested)
	if err != nil {
		t.Fatalf("RelativePathAny 返回错误: %v", err)
	}
	if base != nestedDir {
		t.Fatalf("匹配目录不符合预期，实际 %q", base)
	}
	if rel != "deep.txt" {
		t.Fatalf("相对路径不符合预期，实际 %q", rel)
	}
}

func TestBuildObjectKeyStrictForDirs(t *testing.T) {
	baseDir := t.TempDir()
	nestedDir := filepath.Join(baseDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	fileInNested := filepath.Join(nestedDir, "deep.txt")
	if err := os.WriteFile(fileInNested, []byte("data"), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	key, err := BuildObjectKeyStrictForDirs([]string{baseDir, nestedDir}, fileInNested)
	if err != nil {
		t.Fatalf("构建对象 key 失败: %v", err)
	}
	expected := trimLeadingSlash(joinURLPath(trimLeadingSlash(toSlashPath(nestedDir)), "deep.txt"))
	if key != expected {
		t.Fatalf("严格模式生成的 key 不符合预期: %q（期望 %q）", key, expected)
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

func TestRelativePath_AllowsNonExistentFile(t *testing.T) {
	baseDir := t.TempDir()
	newDir := filepath.Join(baseDir, "new")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	filePath := filepath.Join(newDir, "file.txt")

	rel, err := RelativePath(baseDir, filePath)
	if err != nil {
		t.Fatalf("相对路径计算失败: %v", err)
	}
	if rel != "new/file.txt" {
		t.Fatalf("相对路径不符合预期，实际 %q", rel)
	}
}

func TestSplitWatchDirs_TrimAndDedup(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	raw := "  " + dirA + "  ," + dirA + string(os.PathSeparator) + ";\n" + dirB + " \r " + dirB + string(os.PathSeparator)

	got := SplitWatchDirs(raw)
	want := []string{filepath.Clean(dirA), filepath.Clean(dirB)}
	if len(got) != len(want) {
		t.Fatalf("目录数量不符合预期，实际 %d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("目录内容不符合预期，实际 %v", got)
		}
	}
}

func TestSplitWatchDirs_EmptyInput(t *testing.T) {
	cases := []string{
		"",
		"   ",
		",,,",
		";;\n\r",
		" , ; \n \r ",
	}
	for _, raw := range cases {
		got := SplitWatchDirs(raw)
		if len(got) != 0 {
			t.Fatalf("期望空列表，实际 %v", got)
		}
	}
}

func skipIfSymlinkNotSupported(t *testing.T, err error) {
	t.Helper()
	if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "privilege") {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
}
