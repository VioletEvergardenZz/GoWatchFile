package oss

import (
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
