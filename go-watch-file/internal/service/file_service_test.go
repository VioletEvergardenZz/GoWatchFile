// 本文件用于上传重试相关测试
package service

import (
	"testing"
	"time"

	"file-watch/internal/models"
)

func TestParseUploadRetryDelays(t *testing.T) {
	cases := []struct {
		name string
		cfg  *models.Config
		want []time.Duration
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: defaultUploadRetryDelays,
		},
		{
			name: "empty",
			cfg:  &models.Config{UploadRetryDelays: ""},
			want: defaultUploadRetryDelays,
		},
		{
			name: "valid list",
			cfg:  &models.Config{UploadRetryDelays: "500ms,1s 2s;3s"},
			want: []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 3 * time.Second},
		},
		{
			name: "invalid items fallback",
			cfg:  &models.Config{UploadRetryDelays: "foo,1s,bar"},
			want: []time.Duration{time.Second},
		},
		{
			name: "all invalid fallback default",
			cfg:  &models.Config{UploadRetryDelays: "foo,bar"},
			want: defaultUploadRetryDelays,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseUploadRetryDelays(tc.cfg)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d delays, got %d", len(tc.want), len(got))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("delay[%d] expected %v, got %v", i, tc.want[i], got[i])
				}
			}
		})
	}
}

func TestIsUploadRetryEnabled(t *testing.T) {
	trueVal := true
	falseVal := false
	cases := []struct {
		name string
		cfg  *models.Config
		want bool
	}{
		{name: "nil config", cfg: nil, want: true},
		{name: "nil flag", cfg: &models.Config{}, want: true},
		{name: "enabled", cfg: &models.Config{UploadRetryEnabled: &trueVal}, want: true},
		{name: "disabled", cfg: &models.Config{UploadRetryEnabled: &falseVal}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isUploadRetryEnabled(tc.cfg)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
