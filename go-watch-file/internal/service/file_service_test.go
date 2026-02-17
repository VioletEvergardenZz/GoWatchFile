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

func TestBuildUploadRetryPlan(t *testing.T) {
	cfg := &models.Config{
		UploadRetryDelays:      "1s,2s",
		UploadRetryMaxAttempts: 5,
	}
	plan := buildUploadRetryPlan(cfg)
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	if len(plan) != len(want) {
		t.Fatalf("retry plan expected %d, got %d", len(want), len(plan))
	}
	for i := range plan {
		if plan[i] != want[i] {
			t.Fatalf("retry plan[%d] expected %v, got %v", i, want[i], plan[i])
		}
	}
}

func TestResolveQueueSaturationThreshold(t *testing.T) {
	cases := []struct {
		name string
		cfg  *models.Config
		want float64
	}{
		{name: "nil config", cfg: nil, want: 0.9},
		{name: "zero threshold", cfg: &models.Config{UploadQueueSaturationThreshold: 0}, want: 0.9},
		{name: "invalid threshold", cfg: &models.Config{UploadQueueSaturationThreshold: 1.5}, want: 0.9},
		{name: "valid threshold", cfg: &models.Config{UploadQueueSaturationThreshold: 0.75}, want: 0.75},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveQueueSaturationThreshold(tc.cfg)
			if got != tc.want {
				t.Fatalf("expected %.2f, got %.2f", tc.want, got)
			}
		})
	}
}
