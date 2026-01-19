// 本文件用于排除规则匹配的单元测试
package pathutil

import "testing"

func TestExcludeMatcher_NameOnly(t *testing.T) {
	matcher := NewExcludeMatcher(".git,node_modules")
	if matcher == nil {
		t.Fatalf("matcher should not be nil")
	}
	if !matcher.IsExcluded("/repo/.git/objects") {
		t.Fatalf("expected .git to be excluded")
	}
	if !matcher.IsExcluded("/repo/node_modules/pkg/index.js") {
		t.Fatalf("expected node_modules to be excluded")
	}
	if matcher.IsExcluded("/repo/src/main.go") {
		t.Fatalf("unexpected exclusion for /repo/src/main.go")
	}
}

func TestExcludeMatcher_AbsolutePrefix(t *testing.T) {
	matcher := NewExcludeMatcher("/opt/homebrew")
	if matcher == nil {
		t.Fatalf("matcher should not be nil")
	}
	if !matcher.IsExcluded("/opt/homebrew/bin") {
		t.Fatalf("expected /opt/homebrew/bin to be excluded")
	}
	if !matcher.IsExcluded("/opt/homebrew") {
		t.Fatalf("expected /opt/homebrew to be excluded")
	}
	if matcher.IsExcluded("/opt/home") {
		t.Fatalf("did not expect /opt/home to be excluded")
	}
}

func TestExcludeMatcher_SegmentPattern(t *testing.T) {
	matcher := NewExcludeMatcher("cache/tmp")
	if matcher == nil {
		t.Fatalf("matcher should not be nil")
	}
	if !matcher.IsExcluded("/var/cache/tmp/files") {
		t.Fatalf("expected /var/cache/tmp/files to be excluded")
	}
	if !matcher.IsExcluded("/var/cache/tmp") {
		t.Fatalf("expected /var/cache/tmp to be excluded")
	}
	if matcher.IsExcluded("/var/cache/tmpfile") {
		t.Fatalf("did not expect /var/cache/tmpfile to be excluded")
	}
}

func TestExcludeMatcher_Empty(t *testing.T) {
	if NewExcludeMatcher("") != nil {
		t.Fatalf("expected nil matcher for empty input")
	}
}
