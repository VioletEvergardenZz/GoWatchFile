// 本文件用于知识库评估命令入口 将命中率和 MTTD 评估集中到一个 CLI 便于回归复用

// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type hitrateSample struct {
	Question  string   `json:"question"`
	ExpectAny []string `json:"expectAny"`
}

type searchResponse struct {
	OK    bool `json:"ok"`
	Items []struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Content string `json:"content"`
	} `json:"items"`
}

type askResponse struct {
	OK         bool    `json:"ok"`
	Answer     string  `json:"answer"`
	Confidence float64 `json:"confidence"`
	Citations  []struct {
		ArticleID string `json:"articleId"`
		Title     string `json:"title"`
		Version   int    `json:"version"`
	} `json:"citations"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "hitrate":
		if err := runHitrate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "hitrate failed: %v\n", err)
			os.Exit(1)
		}
	case "citation":
		if err := runCitation(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "citation failed: %v\n", err)
			os.Exit(1)
		}
	case "mttd":
		if err := runMTTD(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "mttd failed: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  kb-eval hitrate -base http://localhost:8082 -samples ../../docs/04-知识库/知识库命中率样本.json [-limit 5]")
	fmt.Println("  kb-eval citation -base http://localhost:8082 -samples ../../docs/04-知识库/知识库命中率样本.json [-limit 3] [-target 1.0]")
	fmt.Println("  kb-eval mttd -input ../../docs/04-知识库/知识库MTTD基线.csv")
}

func runHitrate(args []string) error {
	fs := flag.NewFlagSet("hitrate", flag.ContinueOnError)
	baseURL := fs.String("base", "http://localhost:8082", "api base url")
	samplesPath := fs.String("samples", filepath.FromSlash("../../docs/04-知识库/知识库命中率样本.json"), "samples json path")
	limit := fs.Int("limit", 5, "search result limit")
	timeoutSec := fs.Int("timeout", 8, "request timeout seconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*samplesPath)
	if err != nil {
		return fmt.Errorf("read samples failed: %w", err)
	}
	var samples []hitrateSample
	if err := json.Unmarshal(raw, &samples); err != nil {
		return fmt.Errorf("parse samples failed: %w", err)
	}
	if len(samples) == 0 {
		return fmt.Errorf("samples is empty")
	}

	client := &http.Client{Timeout: time.Duration(*timeoutSec) * time.Second}
	hit := 0
	for i, sample := range samples {
		ok, titles, err := evaluateSample(client, *baseURL, sample, *limit)
		if err != nil {
			return fmt.Errorf("sample %d failed: %w", i+1, err)
		}
		if ok {
			hit++
		}
		fmt.Printf("[%02d] %s => %s\n", i+1, sample.Question, boolLabel(ok))
		if len(titles) > 0 {
			fmt.Printf("     candidates: %s\n", strings.Join(titles, " | "))
		}
	}
	ratio := float64(hit) / float64(len(samples))
	fmt.Printf("\nHitrate: %d/%d = %.2f%%\n", hit, len(samples), ratio*100)
	return nil
}

func evaluateSample(client *http.Client, baseURL string, sample hitrateSample, limit int) (bool, []string, error) {
	payload := map[string]any{
		"query": sample.Question,
		"limit": limit,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/kb/search", bytes.NewBuffer(body))
	if err != nil {
		return false, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed searchResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false, nil, err
	}
	if !parsed.OK {
		return false, nil, fmt.Errorf("search response not ok")
	}
	if len(parsed.Items) == 0 {
		return false, []string{}, nil
	}
	titles := make([]string, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		titles = append(titles, item.Title)
	}
	if len(sample.ExpectAny) == 0 {
		return len(parsed.Items) > 0, titles, nil
	}
	for _, item := range parsed.Items {
		corpus := strings.ToLower(strings.TrimSpace(item.Title + "\n" + item.Summary + "\n" + item.Content))
		for _, keyword := range sample.ExpectAny {
			key := strings.ToLower(strings.TrimSpace(keyword))
			if key == "" {
				continue
			}
			if strings.Contains(corpus, key) {
				return true, titles, nil
			}
		}
	}
	return false, titles, nil
}

func boolLabel(v bool) string {
	if v {
		return "hit"
	}
	return "miss"
}

func runCitation(args []string) error {
	fs := flag.NewFlagSet("citation", flag.ContinueOnError)
	baseURL := fs.String("base", "http://localhost:8082", "api base url")
	samplesPath := fs.String("samples", filepath.FromSlash("../../docs/04-知识库/知识库命中率样本.json"), "samples json path")
	limit := fs.Int("limit", 3, "ask result limit")
	target := fs.Float64("target", 1.0, "required citation ratio 0~1")
	timeoutSec := fs.Int("timeout", 8, "request timeout seconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target < 0 || *target > 1 {
		return fmt.Errorf("target must be between 0 and 1")
	}
	raw, err := os.ReadFile(*samplesPath)
	if err != nil {
		return fmt.Errorf("read samples failed: %w", err)
	}
	var samples []hitrateSample
	if err := json.Unmarshal(raw, &samples); err != nil {
		return fmt.Errorf("parse samples failed: %w", err)
	}
	if len(samples) == 0 {
		return fmt.Errorf("samples is empty")
	}

	client := &http.Client{Timeout: time.Duration(*timeoutSec) * time.Second}
	total := 0
	withCitation := 0
	for i, sample := range samples {
		ok, citationTitles, err := evaluateCitationSample(client, *baseURL, sample.Question, *limit)
		total++
		if err != nil {
			fmt.Printf("[%02d] %s => error: %v\n", i+1, sample.Question, err)
			continue
		}
		if ok {
			withCitation++
		}
		fmt.Printf("[%02d] %s => %s\n", i+1, sample.Question, boolLabel(ok))
		if len(citationTitles) > 0 {
			fmt.Printf("     citations: %s\n", strings.Join(citationTitles, " | "))
		}
	}
	ratio := 0.0
	if total > 0 {
		ratio = float64(withCitation) / float64(total)
	}
	fmt.Printf("\nCitation ratio: %d/%d = %.2f%%\n", withCitation, total, ratio*100)
	fmt.Printf("Target ratio  : %.2f%%\n", *target*100)
	if ratio < *target {
		return fmt.Errorf("citation ratio %.2f%% below target %.2f%%", ratio*100, *target*100)
	}
	return nil
}

func evaluateCitationSample(client *http.Client, baseURL, question string, limit int) (bool, []string, error) {
	payload := map[string]any{
		"question": question,
		"limit":    limit,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/kb/ask", bytes.NewBuffer(body))
	if err != nil {
		return false, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed askResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false, nil, err
	}
	if !parsed.OK {
		return false, nil, fmt.Errorf("ask response not ok")
	}
	titles := make([]string, 0, len(parsed.Citations))
	for _, citation := range parsed.Citations {
		titles = append(titles, citation.Title)
	}
	return len(parsed.Citations) > 0, titles, nil
}

func runMTTD(args []string) error {
	fs := flag.NewFlagSet("mttd", flag.ContinueOnError)
	input := fs.String("input", filepath.FromSlash("../../docs/04-知识库/知识库MTTD基线.csv"), "csv path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	f, err := os.Open(*input)
	if err != nil {
		return fmt.Errorf("open csv failed: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv failed: %w", err)
	}
	if len(rows) <= 1 {
		return fmt.Errorf("csv has no data rows")
	}

	var totalBefore int64
	var totalAfter int64
	count := 0
	for i, row := range rows[1:] {
		if len(row) < 3 {
			return fmt.Errorf("row %d invalid: need 3 columns", i+2)
		}
		before, err := strconv.ParseInt(strings.TrimSpace(row[1]), 10, 64)
		if err != nil {
			return fmt.Errorf("row %d before_mttd_ms invalid: %w", i+2, err)
		}
		after, err := strconv.ParseInt(strings.TrimSpace(row[2]), 10, 64)
		if err != nil {
			return fmt.Errorf("row %d after_mttd_ms invalid: %w", i+2, err)
		}
		if before <= 0 || after <= 0 {
			return fmt.Errorf("row %d mttd must be > 0", i+2)
		}
		totalBefore += before
		totalAfter += after
		count++
	}
	if count == 0 {
		return fmt.Errorf("no valid rows")
	}
	avgBefore := float64(totalBefore) / float64(count)
	avgAfter := float64(totalAfter) / float64(count)
	drop := (avgBefore - avgAfter) / avgBefore
	fmt.Printf("Scenarios: %d\n", count)
	fmt.Printf("Average before: %.2f ms\n", avgBefore)
	fmt.Printf("Average after : %.2f ms\n", avgAfter)
	fmt.Printf("Drop ratio    : %.2f%%\n", drop*100)
	return nil
}
