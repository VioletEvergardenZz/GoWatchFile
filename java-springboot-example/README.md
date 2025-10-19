# OOM 测试应用

## 项目描述
这是一个用于测试 JVM OOM 的 Spring Boot 应用，可以申请指定大小的内存并常驻在堆中，用于模拟内存溢出并生成 hprof 文件。

## 技术栈
- Java 17
- Spring Boot 3.2.0
- Maven
- Lombok
- Docker
- Prometheus
- Grafana

## 快速开始

### 1. 前端界面预览
```bash
# 直接打开静态页面预览
open static/index.html

# 或启动本地服务器
cd static
python3 -m http.server 8080
# 访问 http://localhost:8080
```

### 2. 编译项目
```bash
mvn clean package
```

### 2. 启动应用
```bash
# 使用脚本启动
chmod +x scripts/start-oom-test.sh
./scripts/start-oom-test.sh

# 或直接启动
java -Xms1g -Xmx2g \
  -XX:+HeapDumpOnOutOfMemoryError \
  -XX:HeapDumpPath=./heapdump.hprof \
  -jar target/oom-test-1.0.0.jar
```

### 3. 前端集成状态
```bash
# 前端已集成到 src/main/resources/static/ 目录
# 构建时会自动包含到 JAR 包中
# 无需额外配置，开箱即用
```

### 4. Docker 部署

#### 构建镜像
```bash
# 使用脚本构建
./scripts/docker-build.sh

# 或手动构建
docker build -t oom-test-app:latest .
```

#### 启动服务
```bash
# 使用脚本启动（推荐）
./scripts/docker-run.sh

# 或手动启动
docker-compose up -d
```

#### 查看服务状态
```bash
# 查看所有服务
docker-compose ps

# 查看应用日志
docker-compose logs -f oom-test-app

# 查看监控日志
docker-compose logs -f prometheus
docker-compose logs -f grafana
```

### 5. 监控系统

#### Prometheus 指标
- **JVM 内存指标**: `http://192.168.77.128:8085/actuator/prometheus`
- **Prometheus UI**: `http://192.168.77.128:9090/query`
- **Grafana 仪表板**: `http://192.168.77.128:3000/login` (admin/admin)

#### API 文档
- **Swagger UI**: `http://192.168.77.128:8085/swagger-ui/index.html`
- **OpenAPI JSON**: `http://192.168.77.128:8085/api-docs`
- **API 规范**: 完整的 REST API 文档和测试界面

#### 关键 JVM 指标
- `jvm_memory_used_bytes` - JVM 内存使用量
- `jvm_memory_max_bytes` - JVM 最大内存
- `jvm_gc_collection_seconds` - GC 收集时间
- `jvm_threads_live_threads` - 活跃线程数
- `jvm_classes_loaded_classes` - 已加载类数

#### JVM 监控 Dashboard
为了方便监控 JVM 状态，推荐导入 Grafana 官方 JVM 中文 Dashboard：

**Dashboard ID**: `12856`  
**Dashboard 名称**: JVM (Micrometer) - 中文版  
**导入方式**: 
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

### 6. 测试接口

#### 快速OOM测试
```bash
curl "http://localhost:8085/api/memory/test-oom"
```

#### 申请内存
```bash
# POST方式
curl -X POST "http://localhost:8085/api/memory/allocate" \
  -H "Content-Type: application/json" \
  -d '{"requestId":"test001","sizeMB":100}'

# GET方式
curl "http://localhost:8085/api/memory/allocate?sizeMB=100"
```

#### 批量申请内存
```bash
curl -X POST "http://localhost:8085/api/memory/batch-allocate" \
  -H "Content-Type: application/json" \
  -d '{"sizeMB":100,"count":50}'
```

#### 查看内存统计
```bash
curl "http://localhost:8085/api/memory/stats"
```

#### 释放内存
```bash
curl -X DELETE "http://localhost:8085/api/memory/release/test001"
```

#### 清空所有内存
```bash
curl -X DELETE "http://localhost:8085/api/memory/clear"
```

#### 强制垃圾回收
```bash
curl -X POST "http://localhost:8085/api/memory/gc"
```

#### 部分释放内存
```bash
# 释放指定数量的内存块
curl -X DELETE "http://localhost:8085/api/memory/partial-release?blockCount=5"
```

#### 获取内存详情
```bash
curl "http://localhost:8085/api/memory/details"
```

#### 内存管理测试（推荐）
```bash
# 使用脚本进行完整的内存管理测试
./scripts/memory-management-test.sh

# 或手动测试各个功能
curl "http://localhost:8085/api/memory/allocate?sizeMB=100"  # 申请内存
curl -X POST "http://localhost:8085/api/memory/gc"           # 执行GC
curl -X DELETE "http://localhost:8085/api/memory/clear"      # 清空内存
```

### 前后端集成测试
```bash
# 测试前后端集成是否正常
./scripts/test-frontend-integration.sh

# 构建包含前端的完整 JAR 包
./scripts/build-with-frontend.sh

# 测试集成后的应用
./scripts/test-integrated-app.sh
```

## 前端界面功能

### 控制台特性
- **实时监控仪表板**: JVM 内存使用量、GC 性能指标、线程状态监控
- **内存管理控制台**: 申请内存、批量申请、内存块列表管理、释放操作
- **操作控制面板**: 强制垃圾回收、清空内存、OOM 测试、统计信息
- **操作日志系统**: 实时操作记录、不同级别日志、历史查看

### 界面设计
- 响应式布局，支持桌面端和移动端
- 现代化 UI 设计，渐变色彩和动画效果
- 实时数据更新，每5秒自动刷新监控数据
- 键盘快捷键支持，提升操作效率

### 前后端集成
- **API 集成**: 完全集成 Spring Boot 后端 API
- **实时连接**: 自动检测后端服务状态
- **数据同步**: 前端操作实时同步到后端
- **错误处理**: 完善的错误提示和异常处理
- **JAR 集成**: 前端完全集成到 JAR 包中，无需额外部署

## 接口说明

### 内存申请接口
- **POST** `/api/memory/allocate` - 申请指定大小内存
- **GET** `/api/memory/allocate?sizeMB=100` - 申请100MB内存

### 批量申请接口
- **POST** `/api/memory/batch-allocate` - 批量申请内存
- **GET** `/api/memory/batch-allocate?sizeMB=100&count=50` - 批量申请50个100MB内存块

### 内存管理接口
- **GET** `/api/memory/stats` - 查看内存统计
- **GET** `/api/memory/details` - 查看内存详情
- **DELETE** `/api/memory/release/{requestId}` - 释放指定内存块
- **DELETE** `/api/memory/partial-release?blockCount=N` - 部分释放内存
- **DELETE** `/api/memory/clear` - 清空所有内存
- **POST** `/api/memory/gc` - 强制垃圾回收

### 测试接口
- **GET** `/api/memory/test-oom` - 快速OOM测试
- **GET** `/api/memory/health` - 健康检查

## 配置说明

### JVM参数
- `-Xms1g` - 初始堆内存1GB
- `-Xmx2g` - 最大堆内存2GB
- `-XX:+HeapDumpOnOutOfMemoryError` - OOM时自动生成堆转储
- `-XX:HeapDumpPath=./heapdump.hprof` - 堆转储文件路径

### 应用配置
- **应用端口**: 8080 (Java 应用内部端口)
- **Docker 端口**: 8085 (宿主机访问端口)
- 日志级别：DEBUG
- 日志文件：logs/spring.log

### 端口配置说明
- **直接运行 JAR**: 访问 `http://localhost:8080/`
- **Docker 容器**: 访问 `http://localhost:8085/`
- **前端自动适配**: 无需手动配置端口，自动检测当前访问地址

## 注意事项

1. **生产环境谨慎使用**：此应用会消耗大量内存
2. **监控内存使用**：避免影响系统稳定性
3. **及时清理**：测试完成后及时释放内存
4. **备份重要数据**：OOM可能导致应用崩溃

## 故障排除

### 常见问题
1. **内存不足**：调整JVM堆内存参数
2. **端口占用**：修改application.yml中的端口配置
3. **权限问题**：确保有写入日志和堆转储文件的权限

### 日志查看
```bash
# 查看应用日志
tail -f logs/spring.log

# 查看GC日志
tail -f gc.log
```

## 许可证
MIT License
