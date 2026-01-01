# Java堆内存转储自动化分析系统

> 归档说明：本文件属于旧版 OOM 系统资料，与当前 go-watch-file 代码无直接关联，仅供参考。

## 系统概述

本系统通过DaemonSet部署的Go监控工具，自动监控Java应用程序的堆内存转储文件（hprof），实现自动上传、存储管理、开发人员通知和在线分析报告的完整自动化流程。

## 系统架构

### 核心组件

1. **Java应用程序**：配置堆内存转储参数
2. **DaemonSet监控工具**：基于Go fsnotify的文件监控
3. **对象存储服务**：hprof文件存储和生命周期管理
4. **CMDB配置系统**：应用与开发人员关联关系
5. **通知服务**：内部消息推送接口
6. **Jenkins工作流**：触发MAT-CLI分析任务
7. **MAT-CLI容器**：多版本Java隔离的分析环境
8. **Nginx服务**：在线分析报告展示

### 技术栈

- **监控工具**：Go + fsnotify
- **存储服务**：对象存储（OSS/COS/S3等）
- **部署方式**：Kubernetes DaemonSet
- **文件系统**：HostPath挂载
- **分析工具**：MAT-CLI（Linux命令行版本）
- **Web服务**：Nginx + HTML报告

## 详细配置

### Java应用启动参数

```bash
-XX:+HeapDumpOnOutOfMemoryError
-XX:HeapDumpPath=/logs/dump-${HOSTNAME}-$(date '+%s').hprof
```

**参数说明**：
- `HeapDumpOnOutOfMemoryError`：OOM时自动生成堆转储文件
- `HeapDumpPath`：指定转储文件路径，包含主机名和时间戳

### Kubernetes挂载配置

#### 应用Pod挂载
```yaml
volumes:
- name: logs-volume
  hostPath:
    path: /data/logs/{{ .Release.Namespace }}/{{ .Release.Name }}/
    type: Directory
volumeMounts:
- name: logs-volume
  mountPath: /logs
```

#### DaemonSet监控挂载
```yaml
volumes:
- name: logs-monitor
  hostPath:
    path: /data/logs
    type: Directory
volumeMounts:
- name: logs-monitor
  mountPath: /data/logs
```

## 工作流程

### 1. 文件生成阶段
- Java应用发生OOM异常
- 自动生成hprof转储文件
- 文件保存至`/logs/dump-${HOSTNAME}-${timestamp}.hprof`

### 2. 文件监控阶段
- DaemonSet监控工具持续监听`/data/logs`目录
- 使用Go fsnotify检测新文件创建事件
- 过滤hprof文件类型

### 3. 文件处理阶段
- 自动上传hprof文件至对象存储
- 解析文件绝对路径：`/data/logs/<namespace>/<deployment>/dump-xxx.hprof`
- 提取namespace（$2）和deployment（$3）信息

### 4. 通知推送阶段
- 调用CMDB API查询应用关联的开发人员
- 通过公司内部通知接口推送hprof下载链接
- 包含文件信息、应用信息和下载地址

### 5. 自动化分析阶段
- Go程序触发Jenkins工作流
- Jenkins启动对应Java版本的MAT-CLI容器
- 容器下载对象存储上的hprof文件
- 生成HTML格式的分析报告
- 报告直接挂载到Nginx目录

### 6. 在线展示阶段
- HTML报告直接挂载到Nginx目录
- 通过Nginx提供在线分析报告访问
- 开发人员可直接在浏览器中查看分析结果
- 无需下载hprof文件到本地

### 7. 生命周期管理
- 对象存储配置15天自动删除策略
- 定期清理过期hprof文件和分析报告
- 释放存储空间

## 监控工具特性

### 核心功能
- **实时监控**：基于fsnotify的文件系统事件监听
- **自动上传**：检测到新文件后立即上传至对象存储
- **智能解析**：自动提取应用信息（namespace、deployment）
- **批量处理**：支持并发处理多个文件
- **自动化分析**：集成MAT-CLI工具自动生成报告

### 技术特点
- **高性能**：Go语言实现，低资源消耗
- **高可靠**：支持断点续传和重试机制
- **易扩展**：模块化设计，支持多种存储后端
- **易维护**：完善的日志记录和错误处理
- **全自动化**：从文件检测到报告生成的完整流程

## 配置参数

### 监控配置
```yaml
monitor:
  watchPath: "/data/logs"
  filePattern: "*.hprof"
  scanInterval: "5s"
  maxConcurrent: 10
```

### 存储配置
```yaml
storage:
  type: "oss"  # 支持oss、cos、s3等
  endpoint: "https://oss.example.com"
  bucket: "heapdump-files"
  accessKey: "${ACCESS_KEY}"
  secretKey: "${SECRET_KEY}"
```

### 通知配置
```yaml
notification:
  cmdbApi: "https://cmdb.example.com/api/apps"
  notifyApi: "https://notify.example.com/api/send"
  template: "应用{{appName}}发生OOM异常，请及时分析堆转储文件"
```

### MAT分析配置
```yaml
mat:
  cliPath: "/usr/local/bin/mat-cli"
  outputDir: "/var/www/html/reports"
  maxMemory: "4g"
  reportTypes: ["leak_suspects", "system_overview", "top_components"]
```

## 部署配置

### DaemonSet YAML
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: heapdump-monitor
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app: heapdump-monitor
  template:
    metadata:
      labels:
        app: heapdump-monitor
    spec:
      containers:
      - name: monitor
        image: heapdump-monitor:latest
        volumeMounts:
        - name: logs-monitor
          mountPath: /data/logs
        - name: mat-cli
          mountPath: /usr/local/bin
        env:
        - name: WATCH_PATH
          value: "/data/logs"
        - name: STORAGE_TYPE
          value: "oss"
        - name: MAT_CLI_PATH
          value: "/usr/local/bin/mat-cli"
      volumes:
      - name: logs-monitor
        hostPath:
          path: /data/logs
          type: Directory
      - name: mat-cli
        hostPath:
          path: /usr/local/bin
          type: Directory
```

### Nginx配置
```nginx
server {
    listen 80;
    server_name reports.example.com;
    
    root /var/www/html/reports;
    index index.html;
    
    location / {
        try_files $uri $uri/ =404;
    }
    
    location ~* \.(html|css|js|png|jpg|jpeg|gif|ico)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
}
```

## 监控指标

### 系统指标
- 监控文件数量
- 上传成功率
- 处理延迟时间
- 存储空间使用量
- 分析报告生成成功率

### 业务指标
- 各应用OOM发生频率
- 文件大小分布
- 开发人员响应时间
- 存储成本统计
- 在线报告访问量

## 故障处理

### 常见问题
1. **文件监控失效**：检查DaemonSet状态和日志
2. **上传失败**：验证对象存储配置和网络连接
3. **通知推送失败**：检查CMDB API和通知服务状态
4. **存储空间不足**：确认生命周期策略配置
5. **MAT分析失败**：检查MAT-CLI工具和内存配置
6. **报告生成失败**：验证输出目录权限和空间

### 故障恢复
- 自动重试机制
- 手动文件同步
- 监控服务重启
- 配置参数调整
- 报告重新生成

## 最佳实践

### 部署建议
- 使用DaemonSet确保每个节点都有监控
- 配置资源限制避免资源竞争
- 定期备份监控配置和状态
- 合理配置MAT-CLI内存参数

### 运维建议
- 监控系统运行状态和性能指标
- 定期检查存储空间和文件清理情况
- 及时更新监控工具版本和配置
- 监控在线报告服务的可用性

### 安全建议
- 使用最小权限原则配置访问密钥
- 定期轮换存储服务密钥
- 监控异常访问和操作日志
- 限制报告访问权限

## 总结

本系统通过自动化的方式解决了Java应用OOM问题分析中的文件收集、存储管理、通知推送、自动化分析和在线展示等关键环节，大大提高了问题响应效率，为开发人员提供了便捷的在线问题分析工具。

系统设计充分考虑了Kubernetes环境的特点，通过HostPath挂载实现了跨Pod的文件访问，通过DaemonSet确保了监控的全面覆盖，通过对象存储实现了文件的可靠存储和生命周期管理，通过MAT-CLI实现了自动化分析，通过Nginx提供了便捷的在线访问。

**关键优势**：
1. **全自动化流程**：从文件检测到报告生成的完整自动化
2. **在线分析能力**：开发人员无需下载文件即可在线分析
3. **智能通知系统**：自动识别应用并通知相关开发人员
4. **生命周期管理**：自动清理过期文件，节省存储成本
5. **高可用性**：基于Kubernetes的分布式部署架构
