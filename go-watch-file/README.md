# File Watch Service

一个基于 Go 的文件监控服务，支持文件变化检测、S3 上传、Jenkins 构建触发和企业微信/钉钉通知。

## 项目结构

```
go-watch-file/
├── cmd/                      # 应用程序入口
│   └── main.go               # 主程序入口
├── internal/                 # 内部包
│   ├── config/               # 配置管理
│   │   └── config.go         # 配置加载和验证
│   ├── logger/               # 日志管理
│   │   └── logger.go         # 日志系统
│   ├── s3/                   # S3 客户端
│   │   └── client.go         # S3 上传功能
│   ├── jenkins/              # Jenkins 客户端
│   │   └── client.go         # Jenkins 构建触发
│   ├── wechat/               # 企业微信机器人
│   │   └── robot.go          # 微信消息发送
│   ├── dingtalk/             # 钉钉机器人
│   │   └── robot.go          # 钉钉消息发送
│   ├── watcher/              # 文件监控
│   │   └── file_watcher.go   # 文件变化监控
│   ├── upload/               # 上传管理
│   │   └── worker_pool.go    # 上传工作池
│   ├── service/              # 服务层
│   │   └── file_service.go   # 文件服务协调器
│   ├── pathutil/             # 路径处理
│   │   └── pathutil.go       # 路径规则集中处理
│   └── models/               # 数据模型
│       └── types.go          # 结构体定义
├── pkg/                      # 公共包
│   └── utils/                # 工具函数
│       └── file_utils.go     # 文件操作工具
├── config.yaml               # 配置文件
├── go.mod                    # Go 模块文件
├── go.sum                    # Go 依赖校验文件
└── README.md                 # 项目说明
```

## 模块说明

### 1. cmd/main.go
- 程序入口点
- 命令行参数处理
- 服务初始化和启动
- 信号处理

### 2. internal/config
- 配置文件加载和解析
- 配置验证
- 默认值设置

### 3. internal/logger
- 日志系统初始化
- 多级别日志记录（输出为中文）
- 文件和控制台输出

### 4. internal/s3
- S3 客户端管理
- 文件上传功能
- 下载链接生成

### 5. internal/jenkins
- Jenkins 连接管理
- 构建任务触发

### 6. internal/wechat
- 企业微信机器人
- 消息发送功能

### 7. internal/dingtalk
- 钉钉机器人
- 消息发送功能

### 8. internal/watcher
- 文件系统监控
- 递归目录监控
- 文件变化检测

### 9. internal/upload
- 上传工作池管理
- 并发上传控制
- 队列管理

### 10. internal/service
- 服务协调器
- 业务流程管理
- 组件间通信

### 11. internal/pathutil
- 路径规则集中处理
- 相对路径、S3 Key 与下载 URL 的统一生成

### 12. internal/models
- 配置与统计相关结构体
- 文件事件结构体

### 13. pkg/utils
- 通用工具函数
- 文件操作工具

## 路径处理规则（重要）

为了让上传路径与下载链接更稳定，项目统一采用以下规则：

1. **监控目录是根路径**：所有业务路径都以 `watch_dir` 为基准计算相对路径。
2. **S3 对象 Key**：优先使用相对路径，并移除前导 `/`，统一用 `/` 作为分隔符。
3. **下载链接生成**：
   - `force_path_style: true` 时：`https://endpoint/bucket/objectKey`
   - `force_path_style: false` 时：`https://bucket.endpoint/objectKey`
4. **应用名解析**：取相对路径的第一段目录名作为 `appName`。

示例（Linux 路径）：
```
watch_dir: /data/logs
file:      /data/logs/appA/2025/dump_001.hprof

相对路径: appA/2025/dump_001.hprof
对象 Key: appA/2025/dump_001.hprof
appName : appA
下载链接: https://bucket.endpoint/appA/2025/dump_001.hprof
```

## 使用方法

### 1. 准备环境
- 安装 Go（建议与 `go.mod` 中版本保持一致）
- 准备好 S3 与 Jenkins 的可用配置

### 2. 编译
```bash
go build -o file-watch cmd/main.go
```

### 3. 运行
```bash
./file-watch -config config.yaml
```

### 4. 停止
- 在控制台按 `Ctrl + C`，程序会优雅退出并释放资源

### 5. 日志查看
- `log_file` 为空：日志输出到控制台
- `log_file` 有值：日志同时输出到控制台和文件

### 配置文件示例
```yaml
watch_dir: "/path/to/watch"
file_ext: ".hprof"
robot_key: "your-robot-key"
dingtalk_webhook: "https://oapi.dingtalk.com/robot/send?access_token=your_access_token"
dingtalk_secret: "your-secret"
bucket: "your-bucket"
ak: "your-access-key"
sk: "your-secret-key"
endpoint: "your-s3-endpoint"
region: "your-region"
force_path_style: true
disable_ssl: false
jenkins_host: "http://jenkins.example.com"
jenkins_user: "your-jenkins-user"
jenkins_password: "your-jenkins-password"
jenkins_job: "your-jenkins-job"
log_level: "info"
log_file: "/var/log/file-watch.log"
upload_workers: 3
upload_queue_size: 100
```

## 特性

1. **模块化设计**：清晰的模块分离，便于维护和扩展
2. **并发处理**：使用工作池处理文件上传，提高性能
3. **文件监控**：实时监控文件变化，支持递归目录监控
4. **S3 集成**：支持 S3 兼容存储的文件上传
5. **Jenkins 集成**：自动触发 Jenkins 构建任务
6. **微信/钉钉通知**：企业微信或钉钉机器人消息推送
7. **日志管理**：多级别日志记录，支持文件输出
8. **配置管理**：YAML 配置文件，支持验证和默认值
9. **优雅关闭**：信号处理，确保服务正常关闭

## 开发说明

### 添加新功能
1. 在相应的模块中添加功能
2. 更新 `internal/models` 中的数据结构（如果需要）
3. 在 `internal/service` 中集成新功能
4. 更新配置与文档

### 测试
```bash
go test ./...
```

### 代码规范
- 遵循 Go 语言官方代码规范
- 使用 `gofmt` 格式化代码
- 添加适当的注释和文档
