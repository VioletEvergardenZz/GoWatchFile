# Java堆内存转储自动化分析系统

## 📁 目录结构

```
mat-java-oom-md/
├── README.md                            # 本文件
├── java-heapdump-automation-system.md   # 系统详细文档
├── system-flowchart.md                  # 系统流程图
├── go-watch-file/                       # Go文件监控工具
│   ├── main.go                          # 主程序
│   ├── Dockerfile                       # Docker镜像定义
│   ├── docker-build.sh                  # Docker构建脚本
│   └── Makefile                         # 构建配置
├── ai-analysis/                         # AI智能分析模块
│   ├── main.py                          # AI分析主程序
│   ├── requirements.txt                 # Python依赖
│   ├── Dockerfile                       # Docker镜像定义
│   ├── build.sh                         # 构建脚本
│   └── .dockerignore                    # Docker忽略文件
├── jenkins-job/                         # Jenkins自动化任务
│   ├── jenkinsfile                      # Jenkins流水线
│   ├── dockerfile                       # 内存转储分析镜像
│   ├── entrypoint.sh                    # 容器启动脚本
│   └── dump/                            # 分析报告模板
│       └── index.html                   # 报告首页
└── java-springboot-example/             # 示例应用
    ├── src/                             # Java示例代码
    ├── docker-compose.yml               # 容器编排
    └── prometheus.yml                   # 监控配置
```

## 🚀 系统概述

本系统是一个完整的Java堆内存转储自动化分析解决方案，通过Kubernetes DaemonSet部署的Go监控工具，结合AI智能分析，实现从文件检测到在线分析报告的完整自动化流程。

## ✨ 核心特性

- **🔍 自动监控**：基于Go fsnotify的实时文件监控
- **☁️ 自动存储**：集成对象存储服务，支持多种云存储
- **📊 自动分析**：集成MAT-CLI工具，自动生成HTML分析报告
- **🤖 AI智能分析**：基于大语言模型的深度OOM分析
- **🌐 在线预览**：通过Nginx提供在线分析报告访问
- **🔔 智能通知**：自动识别应用并通知相关开发人员
- **⏰ 生命周期管理**：自动清理过期文件，节省存储成本
- **🔄 CI/CD集成**：Jenkins流水线自动化处理

## 🏗️ 系统架构

### 核心组件

1. **Java应用程序**：配置堆内存转储参数
2. **DaemonSet监控工具**：基于Go fsnotify的文件监控
3. **对象存储服务**：hprof文件存储和生命周期管理
4. **CMDB配置系统**：应用与开发人员关联关系
5. **通知服务**：内部消息推送接口
6. **MAT-CLI工具**：自动化堆内存分析
7. **AI分析引擎**：基于OpenAI API的智能分析
8. **Nginx服务**：在线分析报告展示
9. **Jenkins流水线**：自动化CI/CD处理

### 技术栈

- **监控工具**：Go + fsnotify
- **存储服务**：对象存储（OSS/COS/S3等）
- **部署方式**：Kubernetes DaemonSet
- **文件系统**：HostPath挂载
- **分析工具**：MAT-CLI（Linux命令行版本）
- **AI分析**：Python + OpenAI API
- **Web服务**：Nginx + HTML报告
- **CI/CD**：Jenkins Pipeline

## 📋 工作流程

1. **文件生成** → Java应用OOM异常，自动生成hprof文件
2. **文件监控** → DaemonSet工具监听文件变化
3. **文件处理** → 自动上传至对象存储，解析应用信息
4. **通知推送** → 调用CMDB API，推送通知给开发人员
5. **自动化分析** → 使用MAT-CLI分析hprof文件
6. **AI智能分析** → 基于大语言模型进行深度分析
7. **在线展示** → 生成HTML报告，部署到Nginx
8. **生命周期管理** → 15天后自动清理过期文件

## 🛠️ 快速开始

### 1. 配置Java应用

```bash
-XX:+HeapDumpOnOutOfMemoryError
-XX:HeapDumpPath=/logs/dump-${HOSTNAME}-$(date '+%s').hprof
```

### 2. 部署Go监控工具

```bash
cd go-watch-file
./docker-build.sh
```

### 3. 部署AI分析模块

```bash
cd ai-analysis
./build.sh
```

### 4. 配置Jenkins流水线

```groovy
pipeline {
    agent any
    stages {
        stage('dump') {
            // MAT分析阶段
        }
        stage('ai-analysis') {
            // AI分析阶段
        }
    }
}
```

### 5. 配置对象存储

```yaml
storage:
  type: "oss"
  endpoint: "https://oss.example.com"
  bucket: "heapdump-files"
```

### 6. 配置Nginx服务

```nginx
server {
    listen 80;
    server_name reports.example.com;
    root /var/www/html/reports;
}
```

## 🤖 AI智能分析模块

### 功能特性

- **深度分析**：基于大语言模型的OOM根因分析
- **专业报告**：生成结构化的中文分析报告
- **智能诊断**：自动识别内存泄漏模式和性能瓶颈
- **优化建议**：提供具体的代码和配置优化建议

### 支持的分析内容

1. **根本原因分析** - OOM的根本原因识别
2. **堆内存占用分布** - 内存使用情况分析
3. **对象引用链分析** - 对象引用关系分析
4. **导致OOM的操作链** - 导致内存溢出的操作序列
5. **OOM发生时的堆栈跟踪** - 错误发生时的调用栈
6. **关键词高亮** - 重要信息高亮显示
7. **解决方案与优化建议** - 具体的优化建议

### 使用方法

```bash
# 环境变量配置
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.siliconflow.cn/v1"
export HTML_DIR="/path/to/html/files"
export OUTPUT_FILE="analysis.html"
export MODEL="deepseek-ai/DeepSeek-R1"

# 运行分析
python main.py
```

### Docker部署

```bash
# 构建镜像
docker build -t ai-analysis:v1 .

# 运行容器
docker run --rm \
  -e OPENAI_API_KEY="your-api-key" \
  -e HTML_DIR="/app/input" \
  -e OUTPUT_FILE="/app/output/analysis.html" \
  -v /path/to/html:/app/input:ro \
  -v /path/to/output:/app/output \
  ai-analysis:v1
```

## 📊 监控指标

### 系统指标

- 文件监控数量
- 上传成功率
- 处理延迟时间
- 存储空间使用量
- 分析报告生成成功率
- AI分析成功率

### 业务指标

- 各应用OOM发生频率
- 文件大小分布
- 开发人员响应时间
- 存储成本统计
- 在线报告访问量
- AI分析准确率

### JVM 监控 Dashboard

为了方便监控 Java 应用的 JVM 状态，推荐导入 Grafana 官方 JVM 中文 Dashboard：

**Dashboard ID**: `12856`**Dashboard 名称**: JVM (Micrometer) - 中文版**导入方式**:

1. 在 Grafana 中点击 "+" → "Import"
2. 输入 Dashboard ID: `12856`
3. 选择 Prometheus 数据源
4. 调整应用标签匹配规则
5. 点击 "Import" 完成导入

**主要监控指标**:

- JVM 内存使用情况（堆内存、非堆内存）
- 垃圾回收性能（GC 时间、频率、暂停时间）
- 线程状态（活跃线程、守护线程、峰值线程）
- 类加载统计（已加载类、已卸载类）
- 系统资源使用（CPU、内存、磁盘 I/O）

## 🔧 故障处理

### 常见问题

1. **文件监控失效**：检查DaemonSet状态和日志
2. **上传失败**：验证对象存储配置和网络连接
3. **通知推送失败**：检查CMDB API和通知服务状态
4. **存储空间不足**：确认生命周期策略配置
5. **MAT分析失败**：检查MAT-CLI工具和内存配置
6. **报告生成失败**：验证输出目录权限和空间
7. **AI分析失败**：检查OpenAI API密钥和网络连接

### 故障恢复

- 自动重试机制
- 手动文件同步
- 监控服务重启
- 配置参数调整
- 报告重新生成
- AI分析重新执行

## 📚 文档说明

- **`java-heapdump-automation-system.md`**：系统详细技术文档，包含配置、部署、监控等完整信息
- **`system-flowchart.md`**：系统流程图，包含整体流程、详细工作流、组件交互、数据流向、部署架构和监控指标等图表

## 🎯 最佳实践

### 部署建议

- 使用DaemonSet确保每个节点都有监控
- 配置资源限制避免资源竞争
- 定期备份监控配置和状态
- 合理配置MAT-CLI内存参数
- 设置AI分析超时和重试机制

### 运维建议

- 监控系统运行状态和性能指标
- 定期检查存储空间和文件清理情况
- 及时更新监控工具版本和配置
- 监控在线报告服务的可用性
- 定期评估AI分析准确率

### 安全建议

- 使用最小权限原则配置访问密钥
- 定期轮换存储服务密钥
- 监控异常访问和操作日志
- 限制报告访问权限
- 保护AI API密钥安全

## 🌟 系统优势

1. **全自动化流程**：从文件检测到报告生成的完整自动化
2. **在线分析能力**：开发人员无需下载文件即可在线分析
3. **智能通知系统**：自动识别应用并通知相关开发人员
4. **AI智能分析**：基于大语言模型的深度分析能力
5. **生命周期管理**：自动清理过期文件，节省存储成本
6. **高可用性**：基于Kubernetes的分布式部署架构
7. **CI/CD集成**：Jenkins流水线自动化处理

## 📞 技术支持

如有问题或建议，请联系系统维护团队。

---

**版本**: 2.0.0
**更新时间**: 2025-01-02
**维护团队**: 运维团队
