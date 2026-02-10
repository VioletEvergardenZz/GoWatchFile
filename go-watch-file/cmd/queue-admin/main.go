// 本文件用于上传持久化队列管理命令入口
package main

import (
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
)

type cliOptions struct {
	storePath string
	action    string
	item      string
}

func main() {
	os.Exit(runWithArgs(os.Args[1:], os.Stdout, os.Stderr))
}

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

func parseOptions(args []string, stderr io.Writer) (cliOptions, error) {
	fs := flag.NewFlagSet("queue-admin", flag.ContinueOnError)
	fs.SetOutput(stderr)

	storePath := fs.String("store", "logs/upload-queue.json", "队列存储文件路径")
	action := fs.String("action", "peek", "操作类型：enqueue|dequeue|peek|reset|check|doctor")
	item := fs.String("item", "", "队列元素，action=enqueue 时必填")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法：queue-admin -action <enqueue|dequeue|peek|reset|check|doctor> [-item <value>] [-store <path>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}

	options := cliOptions{
		storePath: strings.TrimSpace(*storePath),
		action:    strings.ToLower(strings.TrimSpace(*action)),
		item:      strings.TrimSpace(*item),
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
		return options, nil
	default:
		fs.Usage()
		return cliOptions{}, fmt.Errorf("不支持的 action: %s", options.action)
	}
}

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
		return handleCheck(queue, stdout)
	case "doctor":
		return handleDoctor(queue, stdout)
	default:
		return exitCodeUsage, fmt.Errorf("不支持的 action: %s", options.action)
	}
}

func handleCheck(queue *persistqueue.FileQueue, stdout io.Writer) (int, error) {
	stats := queue.HealthStats()
	queueSize := len(queue.Items())
	if degraded, reason := checkDegradedReason(stats); degraded {
		fmt.Fprintf(stdout, "status=degraded queueSize=%d store=%s reason=%s\n", queueSize, stats.StoreFile, reason)
		return exitCodeDegraded, nil
	}
	fmt.Fprintf(stdout, "status=ok queueSize=%d store=%s\n", queueSize, stats.StoreFile)
	return exitCodeOK, nil
}

func handleDoctor(queue *persistqueue.FileQueue, stdout io.Writer) (int, error) {
	stats := queue.HealthStats()
	fileExists := false
	fileSizeBytes := int64(0)
	fileModTime := "-"
	if info, err := os.Stat(stats.StoreFile); err == nil {
		fileExists = true
		fileSizeBytes = info.Size()
		fileModTime = info.ModTime().UTC().Format(time.RFC3339)
	} else if !os.IsNotExist(err) {
		return exitCodeStoreErr, fmt.Errorf("读取队列文件状态失败: %w", err)
	}

	queueSize := len(queue.Items())
	fmt.Fprintln(stdout, "doctor report")
	fmt.Fprintf(stdout, "store=%s\n", stats.StoreFile)
	fmt.Fprintf(stdout, "storeExists=%t\n", fileExists)
	fmt.Fprintf(stdout, "storeSizeBytes=%d\n", fileSizeBytes)
	fmt.Fprintf(stdout, "storeModTime=%s\n", fileModTime)
	fmt.Fprintf(stdout, "queueSize=%d\n", queueSize)
	fmt.Fprintf(stdout, "recoveredTotal=%d\n", stats.RecoveredTotal)
	fmt.Fprintf(stdout, "corruptFallbackTotal=%d\n", stats.CorruptFallbackTotal)
	fmt.Fprintf(stdout, "persistWriteFailureTotal=%d\n", stats.PersistWriteFailureTotal)
	if degraded, reason := checkDegradedReason(stats); degraded {
		fmt.Fprintln(stdout, "status=degraded")
		fmt.Fprintf(stdout, "reason=%s\n", reason)
		return exitCodeDegraded, nil
	}
	fmt.Fprintln(stdout, "status=ok")
	return exitCodeOK, nil
}

func checkDegradedReason(stats persistqueue.HealthStats) (bool, string) {
	if stats.CorruptFallbackTotal > 0 {
		return true, "检测到持久化文件损坏降级"
	}
	if stats.PersistWriteFailureTotal > 0 {
		return true, "检测到持久化写失败"
	}
	return false, ""
}
