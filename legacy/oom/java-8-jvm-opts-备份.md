# JVM 参数配置参考手册

> 归档说明：本文件属于旧版 OOM 系统资料，与当前 go-watch-file 代码无直接关联，仅供参考。

**用途**: Java应用程序性能调优与故障排查  
**版本**: 适用于 JDK 8+  
**更新**: 2025-08-22

---

## 📊 参数配置总览

| 参数 | 说明 | 内存区域 | 类别/备注 |
|------|------|----------|-----------|
| **-Xms1000m** | 设置JVM**初始堆内存**为1000MB | 堆内 | 堆大小设置 |
| **-Xmx1000m** | 设置JVM**最大堆内存**为1000MB | 堆内 | 堆大小设置 |
| **-Xmn500m** | 设置**年轻代（Young Generation）**大小为500MB | 堆内（年轻代） | 堆代大小设置 |
| **-Xss256k** | 设置**每个线程的栈内存**大小为256KB | 堆外（线程私有） | 线程栈大小 |
| **-XX:MetaspaceSize=128M** | 设置**元空间**的初始大小为128MB | 堆外（元空间） | 非堆内存 |
| **-XX:MaxMetaspaceSize=256M** | 设置**元空间**的最大大小为256MB，防止无限扩大 | 堆外（元空间） | 非堆内存 |
| **-XX:MaxDirectMemorySize=100M** | 设置**直接内存（Direct Buffer）**的最大可分配容量为100MB | 堆外（直接内存） | 非堆内存 |
| **-XX:-UseAdaptiveSizePolicy** | **关闭**JVM的**自适应大小策略**，需要与下面参数配合固定代大小 | 不适用 | GC策略调优 |
| **-XX:+UseParNewGC** | 年轻代使用**ParNew**垃圾收集器 | 堆内（年轻代） | GC收集器选择 |
| **-XX:+UseConcMarkSweepGC** | 老年代使用**CMS（并发标记清除）**垃圾收集器 | 堆内（老年代） | GC策略调优 |
| **-XX:CMSInitiatingOccupancyFraction=92** | 设置老年代空间使用率达到**92%**时，开始触发CMS回收 | 堆内（老年代） | GC策略调优 |
| **-XX:+UseCMSInitiatingOccupancyOnly** | 强制JVM**仅使用**上面设置的92%阈值来触发CMS，而不是用自适应算法 | 堆内（老年代） | GC策略调优 |
| **-XX:+UseCMSCompactAtFullCollection** | 设置在**Full GC**后进行一次**内存碎片整理** | 堆内（老年代） | GC策略调优 |
| **-XX:CMSFullGCsBeforeCompaction=0** | 设置**每次**Full GC后都进行碎片整理。上一条参数开启后，此参数才有效 | 堆内（老年代） | GC策略调优 |
| **-XX:+CMSParallelInitialMarkEnabled** | 开启CMS**初始标记**阶段的**并行**执行，以降低停顿 | 堆内（老年代） | GC策略调优 |
| **-XX:+CMSScavengeBeforeRemark** | 在CMS的**重新标记（Remark）**阶段之前，先进行一次**年轻代GC** | 堆内 | GC策略调优 |
| **-XX:+HeapDumpOnOutOfMemoryError** | 当发生**OOM（内存溢出）**错误时，自动生成**堆转储（Heap Dump）**文件 | 不适用 | 故障排查 |
| **-XX:HeapDumpPath=/logs/...** | 设置堆转储文件的保存路径和名称模板 | 不适用 | 故障排查 |
| **-XX:+PrintGCDetails** | 在GC日志中**打印详细**的GC信息 | 不适用 | GC日志 |
| **-XX:+PrintGCDateStamps** | 在GC日志中**打印日期时间戳**，而不仅仅是相对于JVM启动的时间 | 不适用 | GC日志 |
| **-Xloggc:/logs/gc.log** | 指定将GC日志输出到`/logs/gc.log`文件中 | 不适用 | GC日志 |

---

## 🎯 参数分类详解

### 1. 堆内存配置 (Heap Memory)

#### 基础堆大小设置
- **`-Xms1000m`**: 初始堆内存，建议设置为最大堆内存的50%-70%
- **`-Xmx1000m`**: 最大堆内存，根据应用实际需求设置，避免过大导致GC停顿

#### 代大小配置
- **`-Xmn500m`**: 年轻代大小，建议为总堆内存的1/3到1/2

### 2. 非堆内存配置 (Non-Heap Memory)

#### 元空间配置
- **`-XX:MetaspaceSize=128M`**: 元空间初始大小
- **`-XX:MaxMetaspaceSize=256M`**: 元空间最大大小，防止无限扩大

#### 直接内存配置
- **`-XX:MaxDirectMemorySize=100M`**: 直接内存最大容量，用于NIO操作

#### 线程栈配置
- **`-Xss256k`**: 每个线程的栈大小，影响并发线程数量

### 3. 垃圾收集器配置 (Garbage Collector)

#### CMS收集器参数
- **`-XX:+UseParNewGC`**: 年轻代使用ParNew收集器
- **`-XX:+UseConcMarkSweepGC`**: 老年代使用CMS收集器
- **`-XX:CMSInitiatingOccupancyFraction=92`**: CMS触发阈值
- **`-XX:+UseCMSInitiatingOccupancyOnly`**: 强制使用固定阈值
- **`-XX:+UseCMSCompactAtFullCollection`**: Full GC后碎片整理
- **`-XX:CMSFullGCsBeforeCompaction=0`**: 每次Full GC后都整理
- **`-XX:+CMSParallelInitialMarkEnabled`**: 并行初始标记
- **`-XX:+CMSScavengeBeforeRemark`**: Remark前年轻代GC

### 4. 监控与故障排查

#### 堆转储配置
- **`-XX:+HeapDumpOnOutOfMemoryError`**: OOM时自动转储
- **`-XX:HeapDumpPath=/logs/...`**: 转储文件路径

#### GC日志配置
- **`-XX:+PrintGCDetails`**: 详细GC信息
- **`-XX:+PrintGCDateStamps`**: 时间戳信息
- **`-Xloggc:/logs/gc.log`**: GC日志文件路径

---

## ⚙️ 配置建议

### 生产环境配置原则

1. **内存配置**
   - 初始堆内存 = 最大堆内存 × 0.7
   - 年轻代大小 = 总堆内存 × 0.4
   - 元空间最大大小设置合理上限

2. **GC配置**
   - 使用CMS收集器降低停顿时间
   - 合理设置CMS触发阈值（85%-95%）
   - 开启并行标记和碎片整理

3. **监控配置**
   - 开启详细GC日志
   - 配置OOM自动转储
   - 设置合理的日志轮转策略

### 常见问题排查

- **频繁Full GC**: 检查堆内存大小和CMS阈值设置
- **内存泄漏**: 使用堆转储文件分析内存占用
- **GC停顿过长**: 调整年轻代大小和收集器参数

---

## 🐳 容器环境内存配置

### Kubernetes 内存资源配置

在容器化环境中，需要合理配置Kubernetes的内存资源限制，确保JVM能够正常运行且不会因为内存不足而被OOM Killer终止。

#### 内存资源计算原则

**总内存需求 = 堆内存 + 非堆内存 + 系统开销**

1. **堆内存 (Heap Memory)**
   - 由 `-Xms` 和 `-Xmx` 参数控制
   - 建议：`-Xmx` 设置为容器内存的 60%-70%

2. **非堆内存 (Non-Heap Memory)**
   - **元空间**: 由 `-XX:MaxMetaspaceSize` 控制
   - **直接内存**: 由 `-XX:MaxDirectMemorySize` 控制
   - **线程栈**: 由 `-Xss` 和线程数量决定
   - 建议：预留 20%-30% 给非堆内存

3. **系统开销**
   - JVM本身开销：约 50-100MB
   - 操作系统开销：约 100-200MB
   - 建议：预留 10%-15% 给系统开销

#### 配置示例

##### 示例1：4GB容器内存配置

```yaml
resources:
  requests:
    memory: "3Gi"    # 请求3GB内存
  limits:
    memory: "4Gi"    # 限制4GB内存
```

**JVM参数配置**:
```bash
-Xms2g -Xmx2.5g -Xmn1g \
-XX:MetaspaceSize=256m \
-XX:MaxMetaspaceSize=512m \
-XX:MaxDirectMemorySize=256m \
-Xss256k
```

**内存分配说明**:
- 堆内存：2.5GB (62.5%)
- 元空间：512MB (12.5%)
- 直接内存：256MB (6.25%)
- 线程栈：假设100个线程 × 256KB = 25MB (0.6%)
- 系统开销：约700MB (17.5%)

##### 示例2：8GB容器内存配置

```yaml
resources:
  requests:
    memory: "6Gi"    # 请求6GB内存
  limits:
    memory: "8Gi"    # 限制8GB内存
```

**JVM参数配置**:
```bash
-Xms4g -Xmx5g -Xmn2g \
-XX:MetaspaceSize=512m \
-XX:MaxMetaspaceSize=1g \
-XX:MaxDirectMemorySize=512m \
-Xss256k
```

**内存分配说明**:
- 堆内存：5GB (62.5%)
- 元空间：1GB (12.5%)
- 直接内存：512MB (6.25%)
- 线程栈：假设200个线程 × 256KB = 50MB (0.6%)
- 系统开销：约1.4GB (17.5%)

#### 最佳实践建议

1. **内存比例分配**
   ```
   堆内存: 60-70%
   非堆内存: 20-30%
   系统开销: 10-15%
   ```

2. **Request vs Limit 设置**
   - **requests**: 设置为实际需要的最小内存（堆内存 + 非堆内存 + 基础开销）
   - **limits**: 设置为最大可用内存，建议不超过物理内存的80%

3. **监控指标**
   - 监控容器内存使用率
   - 监控JVM堆内存使用情况
   - 监控GC频率和停顿时间

4. **故障处理**
   - 配置 `-XX:+HeapDumpOnOutOfMemoryError` 自动生成堆转储
   - 设置合理的 `-XX:HeapDumpPath` 路径
   - 配置内存使用率告警

#### 常见问题与解决方案

| 问题 | 现象 | 解决方案 |
|------|------|----------|
| **容器OOM** | 容器被Kill，日志显示OOM | 增加内存limit，调整JVM参数 |
| **频繁GC** | GC日志显示频繁Full GC | 增加堆内存，调整年轻代大小 |
| **元空间不足** | Metaspace OOM错误 | 增加MaxMetaspaceSize |
| **直接内存不足** | DirectBuffer OOM错误 | 增加MaxDirectMemorySize |

---

## 📝 使用示例

### 基础配置示例
```bash
java -Xms2g -Xmx4g -Xmn1g \
     -XX:MetaspaceSize=256m \
     -XX:MaxMetaspaceSize=512m \
     -XX:+UseParNewGC \
     -XX:+UseConcMarkSweepGC \
     -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=/logs/heapdump.hprof \
     -jar application.jar
```

### 高并发配置示例
```bash
java -Xms4g -Xmx8g -Xmn2g \
     -Xss256k \
     -XX:MetaspaceSize=512m \
     -XX:MaxMetaspaceSize=1g \
     -XX:+UseParNewGC \
     -XX:+UseConcMarkSweepGC \
     -XX:CMSInitiatingOccupancyFraction=85 \
     -XX:+UseCMSInitiatingOccupancyOnly \
     -XX:+UseCMSCompactAtFullCollection \
     -XX:CMSFullGCsBeforeCompaction=0 \
     -XX:+CMSParallelInitialMarkEnabled \
     -XX:+CMSScavengeBeforeRemark \
     -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=/logs/heapdump-%t.hprof \
     -XX:+PrintGCDetails \
     -XX:+PrintGCDateStamps \
     -Xloggc:/logs/gc-%t.log \
     -jar application.jar
```

---

**注意**: 以上参数配置仅供参考，实际使用时请根据应用特性、硬件资源和性能要求进行调整。建议在生产环境部署前进行充分的压力测试和性能调优。
