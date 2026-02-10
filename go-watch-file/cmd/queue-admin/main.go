// 本文件用于上传持久化队列管理命令入口
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"file-watch/internal/persistqueue"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("queue-admin 执行失败: %v", err)
	}
}

func run() error {
	storePath := flag.String("store", "logs/upload-queue.json", "队列存储文件路径")
	action := flag.String("action", "peek", "操作类型：enqueue|dequeue|peek|reset")
	item := flag.String("item", "", "队列元素，action=enqueue 时必填")
	flag.Parse()

	queue, err := persistqueue.NewFileQueue(*storePath)
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(*action)) {
	case "enqueue":
		if strings.TrimSpace(*item) == "" {
			return fmt.Errorf("enqueue 操作必须传入 -item")
		}
		if err := queue.Enqueue(*item); err != nil {
			return err
		}
		fmt.Printf("enqueue ok: %s\n", strings.TrimSpace(*item))
		fmt.Printf("queue size: %d\n", len(queue.Items()))
		return nil
	case "dequeue":
		val, ok, err := queue.Dequeue()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("queue empty")
			return nil
		}
		fmt.Printf("dequeue ok: %s\n", val)
		fmt.Printf("queue size: %d\n", len(queue.Items()))
		return nil
	case "peek":
		items := queue.Items()
		fmt.Printf("queue size: %d\n", len(items))
		for idx, val := range items {
			fmt.Printf("%d. %s\n", idx+1, val)
		}
		return nil
	case "reset":
		if err := queue.Reset(); err != nil {
			return err
		}
		fmt.Println("queue reset ok")
		return nil
	default:
		return fmt.Errorf("不支持的 action: %s", *action)
	}
}
