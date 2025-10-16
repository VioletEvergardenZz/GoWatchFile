# JVM 参数配置参考手册

> **用途**: Java应用程序性能调优与故障排查  
> **版本**: 适用于 JDK 8+  
> **更新**: 2025-08-22  
> **文档状态**: 完整版

---

## 📚 目录

- [参数配置总览](#-参数配置总览)
- [参数分类详解](#-参数分类详解)
- [配置建议](#️-配置建议)
- [容器环境内存配置](#-容器环境内存配置)
- [阿里云SAE最佳实践](#️-阿里云sae最佳实践)
- [阿里云SAE JVM参数配置推荐](#-阿里云sae-jvm参数配置推荐)
- [使用示例](#-使用示例)

---

## 📊 参数配置总览

### 核心参数速查表

| 参数 | 说明 | 内存区域 | 类别/备注 |
|:-----|:-----|:---------|:-----------|
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

#### 1. 内存配置
- **初始堆内存 = 最大堆内存 × 0.7**
- **年轻代大小 = 总堆内存 × 0.4**
- **元空间最大大小设置合理上限**

#### 2. GC配置
- **使用CMS收集器降低停顿时间**
- **合理设置CMS触发阈值（85%-95%）**
- **开启并行标记和碎片整理**

#### 3. 监控配置
- **开启详细GC日志**
- **配置OOM自动转储**
- **设置合理的日志轮转策略**

### 常见问题排查

| 问题类型 | 现象描述 | 排查方向 |
|:---------|:---------|:---------|
| **频繁Full GC** | GC日志显示频繁Full GC | 检查堆内存大小和CMS阈值设置 |
| **内存泄漏** | 使用堆转储文件分析内存占用 | 分析对象引用关系，查找内存泄漏点 |
| **GC停顿过长** | GC停顿时间超过预期 | 调整年轻代大小和收集器参数 |

---

## 🐳 容器环境内存配置

### Kubernetes 内存资源配置

在容器化环境中，需要合理配置Kubernetes的内存资源限制，确保JVM能够正常运行且不会因为内存不足而被OOM Killer终止。

#### 内存资源计算原则

> **总内存需求 = 堆内存 + 非堆内存 + 系统开销**

##### 1. 堆内存 (Heap Memory)
- 由 `-Xms` 和 `-Xmx` 参数控制
- **建议**: `-Xmx` 设置为容器内存的 **60%-70%**

##### 2. 非堆内存 (Non-Heap Memory)
- **元空间**: 由 `-XX:MaxMetaspaceSize` 控制
- **直接内存**: 由 `-XX:MaxDirectMemorySize` 控制
- **线程栈**: 由 `-Xss` 和线程数量决定
- **建议**: 预留 **20%-30%** 给非堆内存

##### 3. 系统开销
- **JVM本身开销**: 约 50-100MB
- **操作系统开销**: 约 100-200MB
- **建议**: 预留 **10%-15%** 给系统开销

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

##### 1. 内存比例分配
```
堆内存: 60-70%
非堆内存: 20-30%
系统开销: 10-15%
```

##### 2. Request vs Limit 设置
- **requests**: 设置为实际需要的最小内存（堆内存 + 非堆内存 + 基础开销）
- **limits**: 设置为最大可用内存，建议不超过物理内存的80%

##### 3. 监控指标
- 监控容器内存使用率
- 监控JVM堆内存使用情况
- 监控GC频率和停顿时间

##### 4. 故障处理
- 配置 `-XX:+HeapDumpOnOutOfMemoryError` 自动生成堆转储
- 设置合理的 `-XX:HeapDumpPath` 路径
- 配置内存使用率告警

#### 常见问题与解决方案

| 问题 | 现象 | 解决方案 |
|:-----|:-----|:---------|
| **容器OOM** | 容器被Kill，日志显示OOM | 增加内存limit，调整JVM参数 |
| **频繁GC** | GC日志显示频繁Full GC | 增加堆内存，调整年轻代大小 |
| **元空间不足** | Metaspace OOM错误 | 增加MaxMetaspaceSize |
| **直接内存不足** | DirectBuffer OOM错误 | 增加MaxDirectMemorySize |

---

## ☁️ 阿里云SAE最佳实践

基于[阿里云SAE JVM内存配置最佳实践](https://help.aliyun.com/zh/sae/serverless-app-engine-classic/use-cases/best-practices-for-jvm-heap-size-configuration)，在容器环境下运行JVM的内存配置需要特别注意避免OOM Killer问题。

### 🎯 推荐配置方式一：使用百分比参数（推荐）

#### 核心参数配置

```bash
-XX:+UseContainerSupport \
-XX:InitialRAMPercentage=70.0 \
-XX:MaxRAMPercentage=70.0 \
-XX:+PrintGCDetails \
-XX:+PrintGCDateStamps \
-Xloggc:/home/admin/nas/gc-${POD_IP}-$(date '+%s').log \
-XX:+HeapDumpOnOutOfMemoryError \
-XX:HeapDumpPath=/home/admin/nas/dump-${POD_IP}-$(date '+%s').hprof
```

#### 参数详细说明

| 参数 | 说明 | 推荐值 | 注意事项 |
|:-----|:-----|:-------|:---------|
| **`-XX:+UseContainerSupport`** | 让JVM检测容器内存大小和处理器数量，而不是整个操作系统 | 必须启用 | 需要JDK 8u191+、JDK 10+ |
| **`-XX:InitialRAMPercentage`** | 设置JVM使用容器内存的初始百分比 | 70.0 | 建议与MaxRAMPercentage保持一致 |
| **`-XX:MaxRAMPercentage`** | 设置JVM使用容器内存的最大百分比 | 70.0 | 最大不超过75.0，为系统组件预留空间 |
| **`-XX:+PrintGCDetails`** | 输出GC详细信息 | 启用 | JDK 11+已废弃，使用`-Xlog:gc`替代 |
| **`-XX:+PrintGCDateStamps`** | 输出GC时间戳（日期形式） | 启用 | JDK 11+已废弃，使用`-Xlog:gc`替代 |
| **`-Xloggc:/path/to/gc.log`** | GC日志文件路径 | 自定义 | 建议挂载到NAS目录实现持久化 |
| **`-XX:+HeapDumpOnOutOfMemoryError`** | OOM时自动生成Dump文件 | 启用 | 便于故障排查 |
| **`-XX:HeapDumpPath=/path/to/dump.hprof`** | Dump文件保存路径 | 自定义 | 建议挂载到NAS目录 |

#### 优势特点

1. **自动适配**: 容器规格调整后，JVM内存自动适配
2. **避免OOM**: 为系统组件预留足够内存空间
3. **灵活配置**: 基于百分比而非固定值，更灵活

### 🔧 推荐配置方式二：固定堆大小参数

#### 核心参数配置

```bash
-Xms2048m -Xmx2048m \
-XX:+PrintGCDetails \
-XX:+PrintGCDateStamps \
-Xloggc:/home/admin/nas/gc-${POD_IP}-$(date '+%s').log \
-XX:+HeapDumpOnOutOfMemoryError \
-XX:HeapDumpPath=/home/admin/nas/dump-${POD_IP}-$(date '+%s').hprof
```

#### 推荐堆大小设置

| 容器内存规格 | JVM堆大小 | 预留比例 | 说明 |
|:-------------|:-----------|:---------|:-----|
| **1 GB** | 600 MB | 40% | 为系统组件预留400MB |
| **2 GB** | 1434 MB | 28.3% | 为系统组件预留566MB |
| **4 GB** | 2867 MB | 28.3% | 为系统组件预留1133MB |
| **8 GB** | 5734 MB | 28.3% | 为系统组件预留2266MB |

#### 注意事项

1. **规格调整**: 容器规格变化时需要重新设置堆大小参数
2. **OOM风险**: 参数设置不合理可能导致容器OOM被强制关闭
3. **系统预留**: 必须为系统组件预留足够内存空间

### 🚨 常见问题与解决方案

#### 1. 容器出现137退出码

**问题现象**: 容器使用内存超过限制，出现容器OOM，导致容器被强制关闭

**原因分析**: 业务应用内存可能并未达到JVM堆大小上限，但容器整体内存超限

**解决方案**: 
- 调小JVM堆大小上限
- 为容器内其他系统组件预留足够内存空间
- 使用百分比参数自动适配

#### 2. 发生OOM却没有生成Dump文件

**问题现象**: 发生OOM Killer时，并不一定会发生JVM OOM，所以不会生成Dump文件

**解决方案**:
- **Java应用**: 适当调小JVM堆内存大小
- **非Java应用**: 调整实例规格，保证充裕的内存资源

#### 3. 堆大小和规格内存参数值设置问题

**重要原则**: 堆大小和规格内存参数值**不能相同**

**原因**: 系统自身组件存在内存开销，如SLS日志收集等

**建议**: 为系统组件预留足够的内存空间

#### 4. JDK 8版本参数设置问题

**问题**: 在JDK 8版本下设置`-XX:MaxRAMPercentage`值为整数时报错

**解决方案**:
- **方式一**: 设置参数值为小数形式，如`70.0`而不是`70`
- **方式二**: 升级JDK版本至JDK 10及以上

**影响参数**: `-XX:InitialRAMPercentage`、`-XX:MinRAMPercentage`同样需要小数形式

#### 5. 内存使用率异常问题

**问题现象**: JVM参数设置了6GB，但是内存使用率却很低

**原因分析**: 操作系统不会马上分配6GB物理内存，需要实际使用后才分配

**正常现象**: 应用启动时内存使用率相对较低，后续会出现攀爬现象

### 📋 版本兼容性说明

#### JDK版本支持情况

| JDK版本 | UseContainerSupport支持 | 日志参数兼容性 | 备注 |
|:---------|:------------------------|:----------------|:-----|
| **JDK 8u191+** | ✅ 支持 | ✅ 兼容 | 推荐使用u191+版本 |
| **JDK 10+** | ✅ 支持 | ✅ 兼容 | 完全支持所有参数 |
| **JDK 11+** | ✅ 支持 | ⚠️ 部分废弃 | 日志参数使用`-Xlog:gc` |
| **Dragonwell 11** | ✅ 支持 | ⚠️ 不支持变量 | 不支持`${POD_IP}`变量 |

#### 日志参数迁移建议

**JDK 11+ 推荐配置**:
```bash
# 替代废弃的日志参数
-Xlog:gc:/home/admin/nas/gc-${POD_IP}-$(date '+%s').log
```

**JDK 8-10 兼容配置**:
```bash
# 传统日志参数
-XX:+PrintGCDetails \
-XX:+PrintGCDateStamps \
-Xloggc:/home/admin/nas/gc-${POD_IP}-$(date '+%s').log
```

### 🎯 最佳实践总结

1. **优先使用百分比参数**: `-XX:MaxRAMPercentage=70.0`
2. **为系统组件预留内存**: 建议预留25%-30%
3. **启用容器感知**: `-XX:+UseContainerSupport`
4. **配置自动转储**: OOM时自动生成堆转储文件
5. **日志持久化**: 挂载到NAS目录或收集到SLS
6. **版本兼容性**: 注意不同JDK版本的参数差异

---

## 📋 阿里云SAE JVM参数配置推荐

基于[阿里云SAE JVM参数配置推荐文档](https://help.aliyun.com/zh/sae/jvm-parameter-configuration-recommend)，以下配置参考值需要结合实际业务场景和压测数据动态调整，确保系统稳定性与性能最优。

### 🎯 调优堆栈内存

#### JVM参数配置参考表

> **注意**: 以下JVM参数仅为参考值，业务上线时最终参数值需根据业务压测结果来设置

| **JVM参数** | **说明** | **1C 2G** | **2C 4G** | **4C 8G** | **8C 16G** |
|:-------------|:---------|:-----------|:-----------|:-----------|:-------------|
| **-Xms** | 初始堆内存大小 | 1G | 2560M | 4G | 10G |
| **-Xmx** | 最大堆内存大小 | 1G | 2560M | 4G | 10G |
| **-Xmn** | 新生代空间大小 | 500M | 1200M | 2G | 5G |
| **-Xss** | 线程堆栈空间大小（JDK 8默认1M） | 1M | 1M | 1M | 1M |
| **-XX:MetaspaceSize** | 初始元空间大小 | 128M | 256M | 384M | 512M |
| **-XX:MaxMetaspaceSize** | 最大元空间大小 | 128M | 256M | 384M | 512M |
| **-XX:MaxDirectMemorySize** | 最大堆外内存大小 | 256M | 256M | 1G | 1G |
| **-XX:ReservedCodeCacheSize** | CodeCache大小 | 64M | 128M | 256M | 256M |

#### 堆栈大小典型配置参数详解

| **配置参数** | **说明** | **示例** |
|:-------------|:---------|:---------|
| **-Xmx** | 设置最大堆大小 | `-Xmx3550m`，设置JVM最大可用内存为3550 MB |
| **-Xms** | 设置JVM初始内存 | `-Xms3550m`，设置JVM初始内存为3550 MB。此值建议与-Xmx相同，避免每次垃圾回收完成后JVM重新分配内存 |
| **-Xmn** | 设置年轻代大小 | `-Xmn2g`，设置年轻代大小为2 GB。整个JVM内存大小=年轻代大小+年老代大小+持久代大小。持久代一般固定大小为64 MB，所以增大年轻代后，将会减小年老代大小。此值对系统性能影响较大，Sun官方推荐配置为整个堆的3/8 |
| **-Xss** | 设置线程的栈大小 | `-Xss128k`，设置每个线程的栈大小为128 KB。**说明** JDK 5.0版本以后每个线程栈大小为1 MB，JDK 5.0以前版本每个线程栈大小为256 KB。请依据应用的线程所需内存大小进行调整。在相同物理内存下，减小该值可以生成更多的线程。但是操作系统对一个进程内的线程个数有一定的限制，无法无限生成，一般在3000个~5000个 |
| **-XX:NewRatio=n** | 设置年轻代和年老代的比值 | `-XX:NewRatio=4`，设置年轻代（包括Eden和两个Survivor区）与年老代的比值（除去持久代）。如果设置为4，那么年轻代与年老代所占比值为1:4，年轻代占整个堆栈的1/5 |
| **-XX:SurvivorRatio=n** | 年轻代中Eden区与两个Survivor区的比值 | `-XX:SurvivorRatio=4`，设置年轻代中Eden区与Survivor区的大小比值。如果设置为4，那么两个Survivor区与一个Eden区的比值为2:4，一个Survivor区占整个年轻代的1/6 |
| **-XX:MaxPermSize=n** | 设置持久代大小 | `-XX:MaxPermSize=16m`，设置持久代大小为16 MB |
| **-XX:MaxTenuringThreshold=n** | 设置垃圾最大年龄 | `-XX:MaxTenuringThreshold=0`，设置垃圾最大年龄。如果设置为0，那么年轻代对象不经过Survivor区，直接进入年老代。对于年老代比较多的应用，提高了效率。如果将此值设置为较大值，那么年轻代对象会在Survivor区进行多次复制，增加了对象在年轻代的存活时间，增加在年轻代即被回收的概率 |

### 🔄 调优回收器GC（Garbage Collection）

#### 吞吐量优先的GC典型配置参数

| **配置参数** | **说明** | **示例** |
|:-------------|:---------|:---------|
| **-XX:+UseParallelGC** | 选择垃圾收集器为并行收集器 | `-Xmx3800m -Xms3800m -Xmn2g -Xss128k -XX:+UseParallelGC -XX:ParallelGCThreads=20`，`-XX:+UseParallelGC`此配置仅对年轻代有效，即在示例配置下，年轻代使用并发收集，而年老代仍旧使用串行收集 |
| **-XX:ParallelGCThreads** | 配置并行收集器的线程数，即同时多少个线程一起进行垃圾回收。**说明** 此值建议配置与处理器数目相等 | `-Xmx3800m -Xms3800m -Xmn2g -Xss128k -XX:+UseParallelGC -XX:ParallelGCThreads=20`，`-XX:ParallelGCThreads=20`表示配置并行收集器的线程数为20个 |
| **-XX:+UseParallelOldGC** | 配置年老代垃圾收集方式为并行收集。**说明** JDK 6.0支持对年老代并行收集 | `-Xmx3550m -Xms3550m -Xmn2g -Xss128k -XX:+UseParallelGC -XX:ParallelGCThreads=20 -XX:+UseParallelOldGC`，`-XX:+UseParallelOldGC`表示对年老代进行并行收集 |
| **-XX:MaxGCPauseMillis** | 设置每次年轻代垃圾回收的最长时间，如果无法满足此时间，JVM会自动调整年轻代大小，以满足此值 | `-Xmx3550m -Xms3550m -Xmn2g -Xss128k -XX:+UseParallelGC -XX:MaxGCPauseMillis=100`，`-XX:MaxGCPauseMillis=100`设置每次年轻代垃圾回收的最长时间为100 ms |
| **-XX:+UseAdaptiveSizePolicy** | 设置此选项后，并行收集器自动选择年轻代区大小和相应的Survivor区比例，以达到目标系统规定的最低响应时间或者收集频率，该值建议使用并行收集器时，并且一直打开 | `-Xmx3550m -Xms3550m -Xmn2g -Xss128k -XX:+UseParallelGC -XX:MaxGCPauseMillis=100 -XX:+UseAdaptiveSizePolicy` |

#### 响应时间优先的GC典型配置参数

| **配置参数** | **说明** | **示例** |
|:-------------|:---------|:---------|
| **-XX:+UseConcMarkSweepGC** | 设置年老代为并发收集。**说明** 配置了`-XX:+UseConcMarkSweepGC`，建议年轻代大小使用`-Xmn`设置 | `-Xmx3550m -Xms3550m -Xmn2g -Xss128k -XX:ParallelGCThreads=20 -XX:+UseConcMarkSweepGC -XX:+UseParNewGC` |
| **-XX:+UseParNewGC** | 设置年轻代为并行收集。可与CMS收集同时使用。JDK 5.0以上版本，JVM根据系统配置自行设置，无需再设置此值 | `-Xmx3550m -Xms3550m -Xmn2g -Xss128k -XX:ParallelGCThreads=20 -XX:+UseConcMarkSweepGC -XX:+UseParNewGC` |
| **-XX:CMSFullGCsBeforeCompaction** | 由于并发收集器不对内存空间进行压缩、整理，所以运行一段时间以后会产生"碎片"，使得运行效率降低。此值设置运行多少次GC以后对内存空间进行压缩、整理 | `-Xmx3550m -Xms3550m -Xmn2g -Xss128k -XX:+UseConcMarkSweepGC -XX:CMSFullGCsBeforeCompaction=5 -XX:+UseCMSCompactAtFullCollection`，`-XX:CMSFullGCsBeforeCompaction=5`，表示运行GC5次后对内存空间进行压缩、整理 |
| **-XX:+UseCMSCompactAtFullCollection** | 打开对年老代的压缩。**说明** 该值可能会影响性能，但是可以消除碎片 | `-Xmx3550m -Xms3550m -Xmn2g -Xss128k -XX:+UseConcMarkSweepGC -XX:CMSFullGCsBeforeCompaction=5 -XX:+UseCMSCompactAtFullCollection` |

#### 用于辅助的GC典型配置参数

| **配置参数** | **说明** |
|:-------------|:---------|
| **-XX:+PrintGC** | 用于输出GC日志 |
| **-XX:+PrintGCDetails** | 用于输出GC日志详情 |
| **-XX:+PrintGCTimeStamps** | 用于输出GC时间戳（JVM启动到当前日期的总时长的时间戳形式）。示例如下：`0.855: [GC (Allocation Failure) [PSYoungGen: 33280K->5118K(38400K)] 33280K->5663K(125952K), 0.0067629 secs] [Times: user=0.01 sys=0.01, real=0.00 secs]` |
| **-XX:+PrintGCDateStamps** | 用于输出GC时间戳（日期形式）。示例如下：`2022-01-27T16:22:20.885+0800: 0.299: [GC pause (G1 Evacuation Pause) (young), 0.0036685 secs]` |
| **-XX:+PrintHeapAtGC** | 在进行GC前后打印出堆的信息 |
| **-Xloggc:../logs/gc.log** | 日志文件的输出路径 |

### 🎯 配置策略选择指南

#### 1. 吞吐量优先场景
**适用场景**: 批处理任务、数据分析、后台计算等对响应时间要求不高的应用

**推荐配置**:
```bash
-Xmx4g -Xms4g -Xmn2g \
-XX:+UseParallelGC \
-XX:ParallelGCThreads=8 \
-XX:+UseParallelOldGC \
-XX:MaxGCPauseMillis=100 \
-XX:+UseAdaptiveSizePolicy
```

#### 2. 响应时间优先场景
**适用场景**: Web应用、API服务、实时交互等对响应时间要求较高的应用

**推荐配置**:
```bash
-Xmx4g -Xms4g -Xmn2g \
-XX:+UseParNewGC \
-XX:+UseConcMarkSweepGC \
-XX:CMSInitiatingOccupancyFraction=85 \
-XX:+UseCMSCompactAtFullCollection \
-XX:CMSFullGCsBeforeCompaction=0
```

#### 3. 混合场景配置
**适用场景**: 既有批处理又有实时交互的混合应用

**推荐配置**:
```bash
-Xmx8g -Xms8g -Xmn3g \
-XX:+UseG1GC \
-XX:MaxGCPauseMillis=200 \
-XX:G1HeapRegionSize=16m \
-XX:G1NewSizePercent=30 \
-XX:G1MaxNewSizePercent=40
```

### 📊 性能调优建议

#### 1. 内存配置调优原则
- **初始堆内存与最大堆内存相等**: 避免运行时内存重新分配
- **年轻代大小**: 建议为总堆内存的1/3到1/2
- **元空间大小**: 根据类加载情况动态调整
- **线程栈大小**: 根据并发线程数量合理设置

#### 2. GC调优原则
- **吞吐量优先**: 使用ParallelGC，关注整体处理能力
- **响应时间优先**: 使用CMS或G1GC，关注停顿时间
- **自适应策略**: 启用UseAdaptiveSizePolicy，让JVM自动调优
- **监控分析**: 通过GC日志分析性能瓶颈

#### 3. 监控与调优流程
1. **基线测试**: 记录当前性能指标
2. **参数调整**: 根据配置建议调整参数
3. **压力测试**: 验证调整后的性能表现
4. **监控分析**: 分析GC日志和性能指标
5. **持续优化**: 根据实际运行情况持续调优

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

### 阿里云SAE推荐配置示例
```bash
java -XX:+UseContainerSupport \
     -XX:InitialRAMPercentage=70.0 \
     -XX:MaxRAMPercentage=70.0 \
     -XX:+PrintGCDetails \
     -XX:+PrintGCDateStamps \
     -Xloggc:/home/admin/nas/gc-${POD_IP}-$(date '+%s').log \
     -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=/home/admin/nas/dump-${POD_IP}-$(date '+%s').hprof \
     -jar application.jar
```

### 阿里云SAE标准配置示例

#### 2C 4G配置示例
```bash
java -Xms2560m -Xmx2560m -Xmn1200m \
     -Xss1m \
     -XX:MetaspaceSize=256m \
     -XX:MaxMetaspaceSize=256m \
     -XX:MaxDirectMemorySize=256m \
     -XX:ReservedCodeCacheSize=128m \
     -XX:+UseParNewGC \
     -XX:+UseConcMarkSweepGC \
     -XX:+PrintGCDetails \
     -XX:+PrintGCDateStamps \
     -Xloggc:/logs/gc.log \
     -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=/logs/heapdump.hprof \
     -jar application.jar
```

#### 4C 8G配置示例
```bash
java -Xms4g -Xmx4g -Xmn2g \
     -Xss1m \
     -XX:MetaspaceSize=384m \
     -XX:MaxMetaspaceSize=384m \
     -XX:MaxDirectMemorySize=1g \
     -XX:ReservedCodeCacheSize=256m \
     -XX:+UseParNewGC \
     -XX:+UseConcMarkSweepGC \
     -XX:+PrintGCDetails \
     -XX:+PrintGCDateStamps \
     -Xloggc:/logs/gc.log \
     -XX:+HeapDumpOnOutOfMemoryError \
     -XX:HeapDumpPath=/logs/heapdump.hprof \
     -jar application.jar
```

---

## 📚 参考资料

- [阿里云SAE JVM内存配置最佳实践](https://help.aliyun.com/zh/sae/serverless-app-engine-classic/use-cases/best-practices-for-jvm-heap-size-configuration)
- [阿里云SAE JVM参数配置推荐](https://help.aliyun.com/zh/sae/jvm-parameter-configuration-recommend)

---

## ⚠️ 注意事项

> **重要提醒**: 以上参数配置仅供参考，实际使用时请根据应用特性、硬件资源和性能要求进行调整。建议在生产环境部署前进行充分的压力测试和性能调优。

---

<div align="center">

**📖 文档版本**: v2.0 | **最后更新**: 2025-08-22 | **维护状态**: 活跃维护中

</div>
