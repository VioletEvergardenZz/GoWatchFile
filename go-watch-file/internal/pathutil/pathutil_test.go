package pathutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRelativePath_PreventsSymlinkEscape(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()

	escapeLink := filepath.Join(baseDir, "escape")
	if err := os.Symlink(outsideDir, escapeLink); err != nil {
		t.Fatalf("symlink create failed: %v", err)
	}

	outsideFile := filepath.Join(outsideDir, "out.txt")
	if err := os.WriteFile(outsideFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("write outside file failed: %v", err)
	}

	_, err := RelativePath(baseDir, filepath.Join(escapeLink, "out.txt"))
	if err == nil || !errors.Is(err, ErrOutsideBaseDir) {
		t.Fatalf("expected outside-dir error, got: %v", err)
	}
}

func TestRelativePath_AllowsSymlinkInside(t *testing.T) {
	baseDir := t.TempDir()
	realDir := filepath.Join(baseDir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	linkDir := filepath.Join(baseDir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink create failed: %v", err)
	}

	targetFile := filepath.Join(realDir, "file.txt")
	if err := os.WriteFile(targetFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	rel, err := RelativePath(baseDir, filepath.Join(linkDir, "file.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != "real/file.txt" {
		t.Fatalf("unexpected relative path, got %q", rel)
	}
}

func TestBuildObjectKey_StrictAndPermissive(t *testing.T) {
	baseDir := t.TempDir()
	fileInDir := filepath.Join(baseDir, "a.txt")
	if err := os.WriteFile(fileInDir, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	key, err := BuildObjectKeyStrict(baseDir, fileInDir)
	if err != nil {
		t.Fatalf("strict build failed: %v", err)
	}
	expectedStrict := trimLeadingSlash(joinURLPath(trimLeadingSlash(toSlashPath(baseDir)), "a.txt"))
	if key != expectedStrict {
		t.Fatalf("unexpected strict key: %q (expected %q)", key, expectedStrict)
	}

	outsideFile := filepath.Join(t.TempDir(), "b.txt")
	if err := os.WriteFile(outsideFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("write outside file failed: %v", err)
	}
	if _, err := BuildObjectKeyStrict(baseDir, outsideFile); !errors.Is(err, ErrOutsideBaseDir) {
		t.Fatalf("expected outside-dir error, got: %v", err)
	}
	permKey := BuildObjectKeyPermissive(baseDir, outsideFile)
	expected := trimLeadingSlash(toSlashPath(outsideFile))
	if permKey != expected {
		t.Fatalf("unexpected permissive key: %q (expected %q)", permKey, expected)
	}
}

func TestBuildDownloadURL_PathStyleAndEscape(t *testing.T) {
	u := BuildDownloadURL("https://example.com:9000/api", "my-bucket", "folder/a b#c?.txt", true, false)
	want := "https://example.com:9000/api/my-bucket/folder/a%20b%23c%3F.txt"
	if u != want {
		t.Fatalf("unexpected download url:\n got: %s\nwant: %s", u, want)
	}
}

func TestBuildDownloadURL_VirtualHostEndpointWithoutScheme(t *testing.T) {
	u := BuildDownloadURL("example.com:9000/base", "bucket", "a b.txt", false, true)
	want := "http://bucket.example.com:9000/base/a%20b.txt"
	if u != want {
		t.Fatalf("unexpected download url:\n got: %s\nwant: %s", u, want)
	}
}
