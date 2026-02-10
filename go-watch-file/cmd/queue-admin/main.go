// 本文件用于上传持久化队列管理命令入口
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"file-watch/internal/persistqueue"
)

const (
	exitCodeOK       = 0
	exitCodeUsage    = 1
	exitCodeStoreErr = 2
	exitCodeDegraded = 3

	outputFormatText = "text"
	outputFormatJSON = "json"
)

type cliOptions struct {
	storePath    string
	action       string
	item         string
	outputFormat string
	strict       bool
}

type doctorReport struct {
	Action                   string `json:"action"`
	Status                   string `json:"status"`
	Reason                   string `json:"reason,omitempty"`
	Store                    string `json:"store"`
	StoreExists              bool   `json:"storeExists"`
	StoreSizeBytes           int64  `json:"storeSizeBytes"`
	StoreModTime             string `json:"storeModTime"`
	QueueSize                int    `json:"queueSize"`
	RecoveredTotal           uint64 `json:"recoveredTotal"`
	CorruptFallbackTotal     uint64 `json:"corruptFallbackTotal"`
	PersistWriteFailureTotal uint64 `json:"persistWriteFailureTotal"`
}

type checkReport struct {
	Action    string `json:"action"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	Store     string `json:"store"`
	QueueSize int    `json:"queueSize"`
}

// main 作为入口函数并串联核心启动流程
func main() {
	os.Exit(runWithArgs(os.Args[1:], os.Stdout, os.Stderr))
}

// runWithArgs 用于执行主流程
func runWithArgs(args []string, stdout io.Writer, stderr io.Writer) int {
	options, err := parseOptions(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "queue-admin 参数错误: %v\n", err)
		return exitCodeUsage
	}
	code, err := execute(options, stdout)
	if err == nil {
		return code
	}
	fmt.Fprintf(stderr, "queue-admin 执行失败: %v\n", err)
	return code
}

// parseOptions 用于解析输入参数或配置
func parseOptions(args []string, stderr io.Writer) (cliOptions, error) {
	fs := flag.NewFlagSet("queue-admin", flag.ContinueOnError)
	fs.SetOutput(stderr)

	storePath := fs.String("store", "logs/upload-queue.json", "队列存储文件路径")
	action := fs.String("action", "peek", "操作类型：enqueue|dequeue|peek|reset|check|doctor")
	item := fs.String("item", "", "队列元素，action=enqueue 时必填")
	outputFormat := fs.String("format", outputFormatText, "输出格式：text|json（仅 check/doctor 支持 json）")
	strict := fs.Bool("strict", false, "严格模式：check/doctor 遇到降级时按失败处理（退出码 2）")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法：queue-admin -action <enqueue|dequeue|peek|reset|check|doctor> [-item <value>] [-store <path>] [-format <text|json>] [-strict]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}

	options := cliOptions{
		storePath:    strings.TrimSpace(*storePath),
		action:       strings.ToLower(strings.TrimSpace(*action)),
		item:         strings.TrimSpace(*item),
		outputFormat: strings.ToLower(strings.TrimSpace(*outputFormat)),
		strict:       *strict,
	}
	if options.storePath == "" {
		fs.Usage()
		return cliOptions{}, fmt.Errorf("-store 不能为空")
	}

	switch options.action {
	case "enqueue", "dequeue", "peek", "reset", "check", "doctor":
		if options.action == "enqueue" && options.item == "" {
			fs.Usage()
			return cliOptions{}, fmt.Errorf("enqueue 操作必须传入 -item")
		}
		if options.outputFormat != outputFormatText && options.outputFormat != outputFormatJSON {
			fs.Usage()
			return cliOptions{}, fmt.Errorf("不支持的 -format: %s", options.outputFormat)
		}
		if options.outputFormat == outputFormatJSON && options.action != "check" && options.action != "doctor" {
			fs.Usage()
			return cliOptions{}, fmt.Errorf("-format=json 仅支持 check/doctor")
		}
		if options.strict && options.action != "check" && options.action != "doctor" {
			fs.Usage()
			return cliOptions{}, fmt.Errorf("-strict 仅支持 check/doctor")
		}
		return options, nil
	default:
		fs.Usage()
		return cliOptions{}, fmt.Errorf("不支持的 action: %s", options.action)
	}
}

// execute 用于分派并执行命令动作
func execute(options cliOptions, stdout io.Writer) (int, error) {
	queue, err := persistqueue.NewFileQueue(options.storePath)
	if err != nil {
		return exitCodeStoreErr, err
	}

	switch options.action {
	case "enqueue":
		if err := queue.Enqueue(options.item); err != nil {
			return exitCodeStoreErr, err
		}
		fmt.Fprintf(stdout, "enqueue ok: %s\n", options.item)
		fmt.Fprintf(stdout, "queue size: %d\n", len(queue.Items()))
		return exitCodeOK, nil
	case "dequeue":
		value, ok, err := queue.Dequeue()
		if err != nil {
			return exitCodeStoreErr, err
		}
		if !ok {
			fmt.Fprintln(stdout, "queue empty")
			return exitCodeOK, nil
		}
		fmt.Fprintf(stdout, "dequeue ok: %s\n", value)
		fmt.Fprintf(stdout, "queue size: %d\n", len(queue.Items()))
		return exitCodeOK, nil
	case "peek":
		items := queue.Items()
		fmt.Fprintf(stdout, "queue size: %d\n", len(items))
		for index, value := range items {
			fmt.Fprintf(stdout, "%d. %s\n", index+1, value)
		}
		return exitCodeOK, nil
	case "reset":
		if err := queue.Reset(); err != nil {
			return exitCodeStoreErr, err
		}
		fmt.Fprintln(stdout, "queue reset ok")
		return exitCodeOK, nil
	case "check":
		return handleCheck(queue, options.outputFormat, options.strict, stdout)
	case "doctor":
		return handleDoctor(queue, options.outputFormat, options.strict, stdout)
	default:
		return exitCodeUsage, fmt.Errorf("不支持的 action: %s", options.action)
	}
}

// handleCheck 用于处理核心流程
func handleCheck(queue *persistqueue.FileQueue, outputFormat string, strict bool, stdout io.Writer) (int, error) {
	report, exitCode := collectCheckReport(queue)
	finalExitCode := normalizeHealthExitCode(exitCode, strict)

	if outputFormat == outputFormatJSON {
		if err := writeJSON(stdout, report); err != nil {
			return exitCodeStoreErr, fmt.Errorf("输出 check JSON 失败: %w", err)
		}
		return finalExitCode, nil
	}

	printCheckReportText(report, stdout)
	return finalExitCode, nil
}

// handleDoctor 用于处理核心流程
func handleDoctor(queue *persistqueue.FileQueue, outputFormat string, strict bool, stdout io.Writer) (int, error) {
	report, exitCode, err := collectDoctorReport(queue)
	if err != nil {
		return exitCodeStoreErr, err
	}
	finalExitCode := normalizeHealthExitCode(exitCode, strict)

	if outputFormat == outputFormatJSON {
		if err := writeJSON(stdout, report); err != nil {
			return exitCodeStoreErr, fmt.Errorf("输出 doctor JSON 失败: %w", err)
		}
		return finalExitCode, nil
	}

	printDoctorReportText(report, stdout)
	return finalExitCode, nil
}

// collectDoctorReport 用于采集并汇总数据
func collectDoctorReport(queue *persistqueue.FileQueue) (doctorReport, int, error) {
	stats := queue.HealthStats()
	fileExists := false
	fileSizeBytes := int64(0)
	fileModTime := "-"
	if info, err := os.Stat(stats.StoreFile); err == nil {
		fileExists = true
		fileSizeBytes = info.Size()
		fileModTime = info.ModTime().UTC().Format(time.RFC3339)
	} else if !os.IsNotExist(err) {
		return doctorReport{}, exitCodeStoreErr, fmt.Errorf("读取队列文件状态失败: %w", err)
	}

	report := doctorReport{
		Action:                   "doctor",
		Status:                   "ok",
		Store:                    stats.StoreFile,
		StoreExists:              fileExists,
		StoreSizeBytes:           fileSizeBytes,
		StoreModTime:             fileModTime,
		QueueSize:                len(queue.Items()),
		RecoveredTotal:           stats.RecoveredTotal,
		CorruptFallbackTotal:     stats.CorruptFallbackTotal,
		PersistWriteFailureTotal: stats.PersistWriteFailureTotal,
	}
	if degraded, reason := checkDegradedReason(stats); degraded {
		report.Status = "degraded"
		report.Reason = reason
		return report, exitCodeDegraded, nil
	}
	return report, exitCodeOK, nil
}

// collectCheckReport 用于采集并汇总数据
func collectCheckReport(queue *persistqueue.FileQueue) (checkReport, int) {
	stats := queue.HealthStats()
	report := checkReport{
		Action:    "check",
		Status:    "ok",
		Store:     stats.StoreFile,
		QueueSize: len(queue.Items()),
	}
	if degraded, reason := checkDegradedReason(stats); degraded {
		report.Status = "degraded"
		report.Reason = reason
		return report, exitCodeDegraded
	}
	return report, exitCodeOK
}

// printCheckReportText 用于输出 check 文本报告
func printCheckReportText(report checkReport, stdout io.Writer) {
	if report.Reason != "" {
		fmt.Fprintf(stdout, "status=%s queueSize=%d store=%s reason=%s\n", report.Status, report.QueueSize, report.Store, report.Reason)
		return
	}
	fmt.Fprintf(stdout, "status=%s queueSize=%d store=%s\n", report.Status, report.QueueSize, report.Store)
}

// printDoctorReportText 用于输出 doctor 文本报告
func printDoctorReportText(report doctorReport, stdout io.Writer) {
	fmt.Fprintln(stdout, "doctor report")
	fmt.Fprintf(stdout, "store=%s\n", report.Store)
	fmt.Fprintf(stdout, "storeExists=%t\n", report.StoreExists)
	fmt.Fprintf(stdout, "storeSizeBytes=%d\n", report.StoreSizeBytes)
	fmt.Fprintf(stdout, "storeModTime=%s\n", report.StoreModTime)
	fmt.Fprintf(stdout, "queueSize=%d\n", report.QueueSize)
	fmt.Fprintf(stdout, "recoveredTotal=%d\n", report.RecoveredTotal)
	fmt.Fprintf(stdout, "corruptFallbackTotal=%d\n", report.CorruptFallbackTotal)
	fmt.Fprintf(stdout, "persistWriteFailureTotal=%d\n", report.PersistWriteFailureTotal)
	fmt.Fprintf(stdout, "status=%s\n", report.Status)
	if report.Reason != "" {
		fmt.Fprintf(stdout, "reason=%s\n", report.Reason)
	}
}

// writeJSON 用于写入数据
func writeJSON(stdout io.Writer, payload any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(payload)
}

// checkDegradedReason 用于判断健康降级原因
func checkDegradedReason(stats persistqueue.HealthStats) (bool, string) {
	if stats.CorruptFallbackTotal > 0 {
		return true, "检测到持久化文件损坏降级"
	}
	if stats.PersistWriteFailureTotal > 0 {
		return true, "检测到持久化写失败"
	}
	return false, ""
}

// normalizeHealthExitCode 用于统一数据格式便于比较与存储
func normalizeHealthExitCode(exitCode int, strict bool) int {
	if strict && exitCode == exitCodeDegraded {
		return exitCodeStoreErr
	}
	return exitCode
}
