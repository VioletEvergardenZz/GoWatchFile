package oss

import (
	"path/filepath"
	"testing"

	"file-watch/internal/models"
)

func TestIsUploadETagVerifyEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  *models.Config
		want bool
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: false,
		},
		{
			name: "disabled",
			cfg: &models.Config{
				UploadETagVerifyEnabled: false,
			},
			want: false,
		},
		{
			name: "enabled",
			cfg: &models.Config{
				UploadETagVerifyEnabled: true,
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isUploadETagVerifyEnabled(tc.cfg)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestNormalizeETag(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "quoted upper",
			raw:  "\"ABCDEF0123456789ABCDEF0123456789\"",
			want: "abcdef0123456789abcdef0123456789",
		},
		{
			name: "plain lower",
			raw:  "abcdef0123456789abcdef0123456789",
			want: "abcdef0123456789abcdef0123456789",
		},
		{
			name: "with spaces",
			raw:  "  \"ABCDEF0123456789ABCDEF0123456789\"  ",
			want: "abcdef0123456789abcdef0123456789",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeETag(tc.raw)
			if got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestIsValidMD5Hex(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "valid",
			raw:  "abcdef0123456789abcdef0123456789",
			want: true,
		},
		{
			name: "too short",
			raw:  "abcdef",
			want: false,
		},
		{
			name: "invalid char",
			raw:  "abcdef0123456789abcdef01234567zz",
			want: false,
		},
		{
			name: "upper char invalid",
			raw:  "ABCDEF0123456789ABCDEF0123456789",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidMD5Hex(tc.raw)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestIsETagMatch(t *testing.T) {
	cases := []struct {
		name   string
		local  string
		remote string
		want   bool
	}{
		{
			name:   "same value with quote",
			local:  "abcdef0123456789abcdef0123456789",
			remote: "\"ABCDEF0123456789ABCDEF0123456789\"",
			want:   true,
		},
		{
			name:   "different value",
			local:  "abcdef0123456789abcdef0123456789",
			remote: "\"abcdef0123456789abcdef0123456788\"",
			want:   false,
		},
		{
			name:   "remote invalid",
			local:  "abcdef0123456789abcdef0123456789",
			remote: "not-md5",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isETagMatch(tc.local, tc.remote)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestIsResumableUploadEnabled(t *testing.T) {
	cases := []struct {
		name     string
		cfg      *models.Config
		fileSize int64
		want     bool
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: false,
		},
		{
			name: "disabled",
			cfg: &models.Config{
				UploadResumableEnabled:   false,
				UploadResumableThreshold: 1024,
			},
			fileSize: 10 * 1024,
			want:     false,
		},
		{
			name: "enabled but below threshold",
			cfg: &models.Config{
				UploadResumableEnabled:   true,
				UploadResumableThreshold: 1024,
			},
			fileSize: 512,
			want:     false,
		},
		{
			name: "enabled and over threshold",
			cfg: &models.Config{
				UploadResumableEnabled:   true,
				UploadResumableThreshold: 1024,
			},
			fileSize: 2048,
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isResumableUploadEnabled(tc.cfg, tc.fileSize)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestResolveUploadResumablePartSize(t *testing.T) {
	cases := []struct {
		name string
		cfg  *models.Config
		want int64
	}{
		{
			name: "nil config default",
			cfg:  nil,
			want: 10 * 1024 * 1024,
		},
		{
			name: "below min use sdk min",
			cfg: &models.Config{
				UploadResumablePartSize: 64 * 1024,
			},
			want: 100 * 1024,
		},
		{
			name: "normal value",
			cfg: &models.Config{
				UploadResumablePartSize: 2 * 1024 * 1024,
			},
			want: 2 * 1024 * 1024,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUploadResumablePartSize(tc.cfg)
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestResolveUploadResumableRoutines(t *testing.T) {
	cases := []struct {
		name string
		cfg  *models.Config
		want int
	}{
		{
			name: "nil config default",
			cfg:  nil,
			want: 1,
		},
		{
			name: "below 1 fallback default",
			cfg: &models.Config{
				UploadResumableRoutines: 0,
			},
			want: 1,
		},
		{
			name: "clamp max",
			cfg: &models.Config{
				UploadResumableRoutines: 200,
			},
			want: 100,
		},
		{
			name: "normal value",
			cfg: &models.Config{
				UploadResumableRoutines: 8,
			},
			want: 8,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUploadResumableRoutines(tc.cfg)
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestResolveUploadResumableThreshold(t *testing.T) {
	cases := []struct {
		name string
		cfg  *models.Config
		want int64
	}{
		{
			name: "nil config default",
			cfg:  nil,
			want: 10 * 1024 * 1024,
		},
		{
			name: "zero fallback default",
			cfg: &models.Config{
				UploadResumableThreshold: 0,
			},
			want: 10 * 1024 * 1024,
		},
		{
			name: "normal value",
			cfg: &models.Config{
				UploadResumableThreshold: 4096,
			},
			want: 4096,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUploadResumableThreshold(tc.cfg)
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestResolveUploadResumableCheckpointDir(t *testing.T) {
	cases := []struct {
		name string
		cfg  *models.Config
		want string
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: "",
		},
		{
			name: "empty",
			cfg: &models.Config{
				UploadResumableCheckpointDir: "   ",
			},
			want: "",
		},
		{
			name: "cleaned",
			cfg: &models.Config{
				UploadResumableCheckpointDir: "logs/upload-checkpoints/../upload-checkpoints",
			},
			want: filepath.Clean("logs/upload-checkpoints/../upload-checkpoints"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUploadResumableCheckpointDir(tc.cfg)
			if got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}
