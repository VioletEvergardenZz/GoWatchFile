# File Watch Service

一个基于Go语言的文件监控服务，支持文件变化检测、S3上传、Jenkins构建触发和企业微信通知。

## 项目结构

```
go-watch-file/
├── cmd/                    # 应用程序入口
│   └── main.go            # 主程序入口
├── internal/              # 内部包
│   ├── config/            # 配置管理
│   │   └── config.go      # 配置加载和验证
│   ├── logger/            # 日志管理
│   │   └── logger.go      # 日志系统
│   ├── s3/                # S3客户端
│   │   └── client.go      # S3上传功能
│   ├── jenkins/           # Jenkins客户端
│   │   └── client.go      # Jenkins构建触发
│   ├── wechat/            # 企业微信机器人
│   │   └── robot.go       # 微信消息发送
│   ├── watcher/           # 文件监控
│   │   └── file_watcher.go # 文件变化监控
│   ├── upload/            # 上传管理
│   │   └── worker_pool.go # 上传工作池
│   ├── service/           # 服务层
│   │   └── file_service.go # 文件服务协调器
│   └── models/            # 数据模型
│       └── types.go       # 结构体定义
├── pkg/                   # 公共包
│   └── utils/             # 工具函数
│       └── file_utils.go  # 文件操作工具
├── config.yaml            # 配置文件
├── go.mod                 # Go模块文件
├── go.sum                 # Go依赖校验文件
└── README.md              # 项目说明
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
- 多级别日志记录
- 文件和控制台输出

### 4. internal/s3
- S3客户端管理
- 文件上传功能
- 下载链接生成

### 5. internal/jenkins
- Jenkins连接管理
- 构建任务触发
- 文件信息解析

### 6. internal/wechat
- 企业微信机器人
- 消息发送功能

### 7. internal/watcher
- 文件系统监控
- 递归目录监控
- 文件变化检测

### 8. internal/upload
- 上传工作池管理
- 并发上传控制
- 队列管理

### 9. internal/service
- 服务协调器
- 业务流程管理
- 组件间通信

### 10. internal/models
- 数据结构定义
- 配置结构体
- 消息结构体

### 11. pkg/utils
- 通用工具函数
- 文件操作工具

## 使用方法

### 编译
```bash
go build -o file-watch cmd/main.go
```

### 运行
```bash
./file-watch -config config.yaml
```

### 配置文件示例
```yaml
watch_dir: "/path/to/watch"
file_ext: ".hprof"
robot_key: "your-robot-key"
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

1. **模块化设计**: 清晰的模块分离，便于维护和扩展
2. **并发处理**: 使用工作池处理文件上传，提高性能
3. **文件监控**: 实时监控文件变化，支持递归目录监控
4. **S3集成**: 支持S3兼容存储的文件上传
5. **Jenkins集成**: 自动触发Jenkins构建任务
6. **微信通知**: 企业微信机器人消息推送
7. **日志管理**: 多级别日志记录，支持文件输出
8. **配置管理**: YAML配置文件，支持验证和默认值
9. **优雅关闭**: 信号处理，确保服务正常关闭

## 开发说明

### 添加新功能
1. 在相应的模块中添加功能
2. 更新models包中的数据结构
3. 在service层中集成新功能
4. 更新配置和文档

### 测试
```bash
go test ./...
```

### 代码规范
- 遵循Go语言官方代码规范
- 使用gofmt格式化代码
- 添加适当的注释和文档
