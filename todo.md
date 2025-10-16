# Java堆内存转储自动化分析系统 - 开发计划

## 📋 项目概述

基于Golang Gin框架开发一个中央管理服务，用于协调和管理整个Java堆内存转储分析系统的各个组件，包括文件监控Agent、Jenkins分析流水线、AI分析模块等。

## 🎯 核心目标

- **中央协调**：统一管理文件监控、分析处理、状态跟踪
- **状态监控**：实时跟踪每个文件的处理状态和进度
- **失败重试**：提供自动和手动重试机制
- **Web界面**：提供直观的管理和监控界面
- **API集成**：与现有组件无缝集成

## 🏗️ 系统架构

### 核心组件
1. **Server服务**：基于Gin框架的中央管理服务
2. **Agent增强**：go-watch-file增加HTTP接口
3. **Jenkins集成**：Server调用Jenkins API
4. **Web界面**：文件状态管理和监控界面
5. **数据库**：存储文件状态和处理记录

### 技术栈
- **后端框架**：Gin + GORM
- **数据库**：MySQL/PostgreSQL
- **前端**：HTML + CSS + JavaScript (Vue.js)
- **消息队列**：Redis (可选)
- **容器化**：Docker + Docker Compose

## 📅 开发阶段规划

### 第一阶段：基础架构搭建 (2-3周)

#### 1.1 Server服务基础框架
- [ ] 创建Gin项目结构
- [ ] 配置数据库连接和模型
- [ ] 实现基础的路由和中间件
- [ ] 添加日志和错误处理
- [ ] 配置环境变量和配置文件

#### 1.2 数据库设计
- [ ] 设计文件状态表结构
- [ ] 设计处理记录表结构
- [ ] 设计重试记录表结构
- [ ] 创建数据库迁移脚本
- [ ] 添加索引和约束

#### 1.3 基础API接口
- [ ] 文件创建通知接口
- [ ] 文件上传开始通知接口
- [ ] 文件上传完成通知接口
- [ ] Jenkins分析完成通知接口
- [ ] 文件状态查询接口

### 第二阶段：Agent集成 (2-3周)

#### 2.1 go-watch-file增强
- [ ] 添加HTTP客户端配置
- [ ] 实现文件创建通知功能
- [ ] 实现上传开始通知功能
- [ ] 实现上传完成通知功能
- [ ] 添加重试机制和错误处理
- [ ] 添加HTTP接口供Server调用

#### 2.2 Agent HTTP接口
- [ ] 文件重试上传接口
- [ ] 文件状态查询接口
- [ ] Agent健康检查接口
- [ ] 配置更新接口
- [ ] 日志查询接口

#### 2.3 Server与Agent集成
- [ ] 实现Agent注册机制
- [ ] 实现心跳检测
- [ ] 实现配置同步
- [ ] 实现远程控制功能

### 第三阶段：Jenkins集成 (2-3周)

#### 3.1 Jenkins API集成
- [ ] 研究Jenkins REST API
- [ ] 实现Jenkins任务触发接口
- [ ] 实现任务状态查询接口
- [ ] 实现任务结果获取接口
- [ ] 添加Jenkins认证和授权

#### 3.2 分析流程管理
- [ ] 实现MAT分析任务管理
- [ ] 实现AI分析任务管理
- [ ] 实现任务状态跟踪
- [ ] 实现任务结果处理
- [ ] 添加任务超时和重试机制

#### 3.3 报告链接管理
- [ ] 实现报告链接生成
- [ ] 实现链接有效性检查
- [ ] 实现链接访问统计
- [ ] 实现链接过期管理

### 第四阶段：Web界面开发 (3-4周)

#### 4.1 前端框架搭建
- [ ] 选择前端框架 (Vue.js/React)
- [ ] 搭建项目结构
- [ ] 配置构建工具
- [ ] 设计UI组件库
- [ ] 实现响应式布局

#### 4.2 核心页面开发
- [ ] 文件列表页面
- [ ] 文件详情页面
- [ ] 状态监控页面
- [ ] 重试管理页面
- [ ] 系统配置页面

#### 4.3 功能模块开发
- [ ] 实时状态更新
- [ ] 文件搜索和过滤
- [ ] 批量操作功能
- [ ] 导出功能
- [ ] 权限管理

### 第五阶段：高级功能 (2-3周)

#### 5.1 监控和告警
- [ ] 实现系统监控指标
- [ ] 实现告警规则配置
- [ ] 实现告警通知功能
- [ ] 实现监控仪表板
- [ ] 集成Prometheus/Grafana

#### 5.2 性能优化
- [ ] 数据库查询优化
- [ ] 缓存机制实现
- [ ] 并发处理优化
- [ ] 内存使用优化
- [ ] 接口性能优化

#### 5.3 安全加固
- [ ] 实现API认证
- [ ] 实现权限控制
- [ ] 实现数据加密
- [ ] 实现审计日志
- [ ] 实现安全扫描

### 第六阶段：测试和部署 (2-3周)

#### 6.1 测试开发
- [ ] 单元测试编写
- [ ] 集成测试编写
- [ ] 端到端测试编写
- [ ] 性能测试编写
- [ ] 安全测试编写

#### 6.2 部署准备
- [ ] Docker镜像构建
- [ ] Kubernetes配置
- [ ] 环境配置管理
- [ ] 备份恢复方案
- [ ] 监控告警配置

#### 6.3 文档编写
- [ ] API文档编写
- [ ] 部署文档编写
- [ ] 用户手册编写
- [ ] 运维手册编写
- [ ] 故障处理手册

## 📊 数据库设计

### 文件状态表 (file_status)
```sql
CREATE TABLE file_status (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    file_name VARCHAR(255) NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    file_size BIGINT,
    app_name VARCHAR(100),
    namespace VARCHAR(100),
    node_name VARCHAR(100) NOT NULL,
    status ENUM('created', 'uploading', 'uploaded', 'analyzing', 'analyzed', 'failed') DEFAULT 'created',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_status (status),
    INDEX idx_app_name (app_name),
    INDEX idx_node_name (node_name),
    INDEX idx_created_at (created_at)
);
```

### 处理记录表 (processing_records)
```sql
CREATE TABLE processing_records (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    file_id BIGINT NOT NULL,
    step_name VARCHAR(100) NOT NULL,
    step_status ENUM('pending', 'running', 'success', 'failed') DEFAULT 'pending',
    started_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES file_status(id)
);
```

### 重试记录表 (retry_records)
```sql
CREATE TABLE retry_records (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    file_id BIGINT NOT NULL,
    retry_type ENUM('upload', 'analysis', 'ai_analysis') NOT NULL,
    retry_reason VARCHAR(500),
    retry_status ENUM('pending', 'running', 'success', 'failed') DEFAULT 'pending',
    retry_count INT DEFAULT 0,
    max_retry_count INT DEFAULT 3,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES file_status(id)
);
```

## 🔌 API接口设计

### 文件状态管理接口
```
GET    /api/v1/files                    # 获取文件列表
GET    /api/v1/files/:id                # 获取文件详情
POST   /api/v1/files/:id/retry          # 重试文件处理
GET    /api/v1/files/:id/status         # 获取文件状态
PUT    /api/v1/files/:id/status         # 更新文件状态
GET    /api/v1/files/node/:node_name    # 获取指定节点的文件列表
```

### Agent管理接口
```
GET    /api/v1/agents                   # 获取Agent列表
GET    /api/v1/agents/node/:node_name   # 获取指定节点的Agent信息
POST   /api/v1/agents/:id/retry         # 重试Agent任务
GET    /api/v1/agents/:id/health        # Agent健康检查
POST   /api/v1/agents/:id/config        # 更新Agent配置
```

### Jenkins管理接口
```
POST   /api/v1/jenkins/trigger          # 触发Jenkins任务
GET    /api/v1/jenkins/status/:job_id   # 获取任务状态
POST   /api/v1/jenkins/retry/:job_id    # 重试Jenkins任务
GET    /api/v1/jenkins/logs/:job_id     # 获取任务日志
```

### 报告管理接口
```
GET    /api/v1/reports/:file_id         # 获取报告链接
POST   /api/v1/reports/:file_id/refresh # 刷新报告链接
GET    /api/v1/reports/stats            # 获取报告统计
```

## 🎨 Web界面设计

### 主要页面
1. **仪表板页面**
   - 系统概览统计
   - 实时状态监控
   - 最近处理文件
   - 告警信息展示

2. **文件管理页面**
   - 文件列表展示
   - 状态筛选和搜索
   - 批量操作功能
   - 详细信息查看

3. **监控页面**
   - 处理流程监控
   - 性能指标展示
   - 错误日志查看
   - 系统健康状态

4. **重试管理页面**
   - 失败任务列表
   - 重试操作界面
   - 重试历史记录
   - 重试策略配置

5. **配置管理页面**
   - Agent配置管理
   - Jenkins配置管理
   - 系统参数配置
   - 告警规则配置

## 🔄 工作流程

### 文件处理流程
1. **文件创建** → Agent发送创建通知(包含节点信息) → Server记录状态
2. **开始上传** → Agent发送上传开始通知 → Server更新状态
3. **上传完成** → Agent发送上传完成通知 → Server触发Jenkins
4. **Jenkins分析** → Jenkins完成分析 → Server更新状态
5. **报告生成** → 生成报告链接 → Server记录报告信息

### 重试流程
1. **失败检测** → 系统检测到处理失败
2. **重试策略** → 根据配置决定重试方式
3. **重试执行** → 根据节点信息调用对应Agent进行重试
4. **状态更新** → 更新处理状态和重试记录
5. **告警通知** → 重试失败时发送告警

## 📈 监控指标

### 系统指标
- 文件处理成功率
- 平均处理时间
- 重试成功率
- 系统响应时间
- 数据库连接数
- 内存使用率

### 业务指标
- 每日处理文件数
- 各应用OOM频率
- 各节点处理文件数
- 分析报告生成率
- 用户访问统计
- 错误类型分布

### 告警指标
- 处理失败率超过阈值
- 系统响应时间过长
- 数据库连接异常
- Agent离线告警
- 特定节点处理异常
- Jenkins任务失败

## 🚀 部署方案

### 开发环境
- Docker Compose本地部署
- 使用SQLite数据库
- 简化配置和依赖

### 测试环境
- Kubernetes集群部署
- 使用MySQL数据库
- 完整功能测试

### 生产环境
- 高可用Kubernetes部署
- 数据库主从配置
- 负载均衡和监控

## 📚 技术文档

### 开发文档
- [ ] 项目架构设计文档
- [ ] API接口设计文档
- [ ] 数据库设计文档
- [ ] 前端组件设计文档
- [ ] 部署架构文档

### 运维文档
- [ ] 部署指南
- [ ] 配置说明
- [ ] 监控配置
- [ ] 故障处理手册
- [ ] 性能调优指南

## 🎯 里程碑

### 里程碑1：基础框架完成 (第3周)
- Server基础服务可运行
- 数据库模型设计完成
- 基础API接口可用

### 里程碑2：Agent集成完成 (第6周)
- go-watch-file增强完成
- Server与Agent通信正常
- 文件状态跟踪准确

### 里程碑3：Jenkins集成完成 (第9周)
- Jenkins API集成完成
- 分析流程管理正常
- 报告链接生成可用

### 里程碑4：Web界面完成 (第13周)
- 核心页面开发完成
- 用户交互功能正常
- 响应式设计适配

### 里程碑5：系统上线 (第16周)
- 所有功能开发完成
- 测试通过
- 生产环境部署成功

## 🔧 开发工具和环境

### 开发工具
- **IDE**: GoLand / VS Code
- **版本控制**: Git
- **API测试**: Postman / Insomnia
- **数据库管理**: DBeaver / Navicat
- **前端开发**: Vue CLI / Create React App

### 开发环境
- **Go版本**: 1.21+
- **Node.js版本**: 18+
- **数据库**: MySQL 8.0+ / PostgreSQL 14+
- **缓存**: Redis 6.0+
- **容器**: Docker 20.10+

## 📝 注意事项

### 开发注意事项
1. **代码规范**: 遵循Go和前端代码规范
2. **错误处理**: 完善的错误处理和日志记录
3. **性能考虑**: 注意数据库查询和接口性能
4. **安全考虑**: 实现适当的认证和授权
5. **可扩展性**: 设计时考虑未来功能扩展

### 测试注意事项
1. **单元测试**: 核心业务逻辑必须有单元测试
2. **集成测试**: 组件间集成需要充分测试
3. **性能测试**: 关键接口需要进行性能测试
4. **安全测试**: 进行安全漏洞扫描
5. **兼容性测试**: 确保多浏览器兼容

---

**预计总开发时间**: 16-20周  
**团队规模建议**: 2-3人  
**优先级**: 高  
**状态**: 待开始
