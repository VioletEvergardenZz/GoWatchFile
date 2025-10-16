package com.example.oomtest.service;

import com.example.oomtest.dto.*;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.time.LocalDateTime;
import java.time.ZoneId;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicInteger;

@Service
@Slf4j
public class MemoryService {
    
    // 存储所有申请的内存块，防止被GC回收
    private final ConcurrentHashMap<String, MemoryBlock> memoryBlocks = new ConcurrentHashMap<>();
    
    // 记录总内存使用量
    private final AtomicLong totalMemoryUsed = new AtomicLong(0);
    
    // 记录内存块数量
    private final AtomicLong blockCount = new AtomicLong(0);
    
    // 记录申请次数
    private final AtomicInteger allocationCount = new AtomicInteger(0);
    
    // 新增：可回收内存相关统计
    private final AtomicLong totalGcableMemoryUsed = new AtomicLong(0);
    private final AtomicLong gcableBlockCount = new AtomicLong(0);
    private final AtomicInteger gcableAllocationCount = new AtomicInteger(0);
    
    // 新增：GC 效果跟踪
    private final AtomicLong residentMemoryBeforeGC = new AtomicLong(0);
    private final AtomicLong gcableMemoryBeforeGC = new AtomicLong(0);
    
    // 内存块记录类（使用record，Java 17特性）
    public record MemoryBlock(
        byte[] data,
        int sizeMB,
        String requestId,
        Instant timestamp
    ) {
        public MemoryBlock {
            // 验证参数
            if (data == null || data.length == 0) {
                throw new IllegalArgumentException("数据不能为空");
            }
            if (sizeMB <= 0) {
                throw new IllegalArgumentException("内存大小必须大于0");
            }
            if (requestId == null || requestId.trim().isEmpty()) {
                throw new IllegalArgumentException("请求ID不能为空");
            }
        }
        
        // 获取时间戳的本地时间表示
        public LocalDateTime getLocalDateTime() {
            return LocalDateTime.ofInstant(timestamp, ZoneId.systemDefault());
        }
    }
    
    /**
     * 申请指定大小的内存并常驻在堆中
     */
    public MemoryAllocationResult allocateMemory(String requestId, int sizeMB) {
        try {
            // 转换为字节
            long sizeBytes = (long) sizeMB * 1024 * 1024;
            
            log.info("开始申请内存 - 请求ID: {}, 大小: {}MB, 当前已用: {}MB", 
                    requestId, sizeMB, totalMemoryUsed.get() / (1024 * 1024));
            
            // 创建指定大小的字节数组
            byte[] memoryBlock = new byte[(int) sizeBytes];
            
            // 填充数据，确保内存被实际使用
            fillMemoryWithData(memoryBlock);
            
            // 创建MemoryBlock记录
            MemoryBlock block = new MemoryBlock(memoryBlock, sizeMB, requestId, Instant.now());
            
            // 存储到内存映射中，防止被GC回收
            memoryBlocks.put(requestId, block);
            
            // 更新统计信息
            totalMemoryUsed.addAndGet(sizeBytes);
            long currentBlockCount = blockCount.incrementAndGet();
            int currentAllocationCount = allocationCount.incrementAndGet();
            
            // 强制触发一次GC，观察内存使用情况
            if (currentAllocationCount % 5 == 0) {
                System.gc();
                log.info("触发GC - 当前内存使用: {}MB", totalMemoryUsed.get() / (1024 * 1024));
            }
            
            log.info("内存申请成功 - 请求ID: {}, 大小: {}MB, 总内存: {}MB, 块数量: {}, 申请次数: {}", 
                    requestId, sizeMB, totalMemoryUsed.get() / (1024 * 1024), currentBlockCount, currentAllocationCount);
            
            return MemoryAllocationResult.builder()
                    .success(true)
                    .requestId(requestId)
                    .allocatedSizeMB(sizeMB)
                    .totalMemoryUsedMB(totalMemoryUsed.get() / (1024 * 1024))
                    .blockCount(currentBlockCount)
                    .allocationCount(currentAllocationCount)
                    .message("内存申请成功")
                    .build();
                    
        } catch (OutOfMemoryError e) {
            log.error("内存申请失败 - 请求ID: {}, 大小: {}MB, 错误: {}", requestId, sizeMB, e.getMessage());
            
            // 记录OOM时的详细信息
            Runtime runtime = Runtime.getRuntime();
            long maxMemory = runtime.maxMemory();
            long totalMemory = runtime.totalMemory();
            long freeMemory = runtime.freeMemory();
            long usedMemory = totalMemory - freeMemory;
            
            log.error("OOM发生时内存状态 - 最大内存: {}MB, 已分配: {}MB, 已使用: {}MB, 空闲: {}MB", 
                    maxMemory / (1024 * 1024), totalMemory / (1024 * 1024), 
                    usedMemory / (1024 * 1024), freeMemory / (1024 * 1024));
            
            return MemoryAllocationResult.builder()
                    .success(false)
                    .requestId(requestId)
                    .allocatedSizeMB(sizeMB)
                    .totalMemoryUsedMB(totalMemoryUsed.get() / (1024 * 1024))
                    .blockCount(blockCount.get())
                    .allocationCount(allocationCount.get())
                    .message("内存申请失败: " + e.getMessage())
                    .build();
        }
    }
    
    /**
     * 申请可回收内存（不保存引用，可以被GC回收）
     */
    public GcableMemoryAllocationResult allocateGcableMemory(String requestId, int sizeMB, boolean releaseImmediately) {
        try {
            // 转换为字节
            long sizeBytes = (long) sizeMB * 1024 * 1024;
            
            log.info("开始申请可回收内存 - 请求ID: {}, 大小: {}MB, 立即释放: {}, 当前可回收: {}MB", 
                    requestId, sizeMB, releaseImmediately, totalGcableMemoryUsed.get() / (1024 * 1024));
            
            // 创建指定大小的字节数组
            byte[] memoryBlock = new byte[(int) sizeBytes];
            
            // 填充数据，确保内存被实际使用
            fillMemoryWithData(memoryBlock);
            
            // 创建MemoryBlock记录（仅用于统计，不保存引用）
            MemoryBlock block = new MemoryBlock(memoryBlock, sizeMB, requestId, Instant.now());
            
            // 更新统计信息
            totalGcableMemoryUsed.addAndGet(sizeBytes);
            long currentGcableBlockCount = gcableBlockCount.incrementAndGet();
            int currentGcableAllocationCount = gcableAllocationCount.incrementAndGet();
            
            // 如果选择立即释放引用，则让内存块可以被GC回收
            if (releaseImmediately) {
                // 不保存引用，让内存块自然进入GC范围
                log.info("立即释放引用 - 内存块将可以被GC回收");
            }
            
            // 强制触发一次GC，观察内存使用情况
            if (currentGcableAllocationCount % 5 == 0) {
                System.gc();
                log.info("触发GC - 当前可回收内存: {}MB", totalGcableMemoryUsed.get() / (1024 * 1024));
            }
            
            log.info("可回收内存申请成功 - 请求ID: {}, 大小: {}MB, 总可回收: {}MB, 块数量: {}, 申请次数: {}", 
                    requestId, sizeMB, totalGcableMemoryUsed.get() / (1024 * 1024), currentGcableBlockCount, currentGcableAllocationCount);
            
            return GcableMemoryAllocationResult.builder()
                    .success(true)
                    .requestId(requestId)
                    .allocatedSizeMB(sizeMB)
                    .totalGcableMemoryMB(totalGcableMemoryUsed.get() / (1024 * 1024))
                    .totalResidentMemoryMB(totalMemoryUsed.get() / (1024 * 1024))
                    .gcableBlockCount(currentGcableBlockCount)
                    .residentBlockCount(blockCount.get())
                    .message("可回收内存申请成功")
                    .referenceReleased(releaseImmediately)
                    .build();
                    
        } catch (OutOfMemoryError e) {
            log.error("可回收内存申请失败 - 请求ID: {}, 大小: {}MB, 错误: {}", requestId, sizeMB, e.getMessage());
            
            return GcableMemoryAllocationResult.builder()
                    .success(false)
                    .requestId(requestId)
                    .allocatedSizeMB(sizeMB)
                    .totalGcableMemoryMB(totalGcableMemoryUsed.get() / (1024 * 1024))
                    .totalResidentMemoryMB(totalMemoryUsed.get() / (1024 * 1024))
                    .gcableBlockCount(gcableBlockCount.get())
                    .residentBlockCount(blockCount.get())
                    .message("可回收内存申请失败: " + e.getMessage())
                    .referenceReleased(false)
                    .build();
        }
    }
    
    /**
     * 填充内存数据，防止被JVM优化
     */
    private void fillMemoryWithData(byte[] memoryBlock) {
        // 使用Java 17的增强for循环和模式匹配
        for (int i = 0; i < memoryBlock.length; i++) {
            // 使用不同的填充策略
            memoryBlock[i] = switch (i % 4) {
                case 0 -> (byte) (i % 256);
                case 1 -> (byte) ((i * 7) % 256);
                case 2 -> (byte) ((i * 13) % 256);
                case 3 -> (byte) ((i * 17) % 256);
                default -> (byte) (i % 256);
            };
        }
        
        // 在内存块末尾添加一些特殊标记
        if (memoryBlock.length >= 8) {
            memoryBlock[memoryBlock.length - 8] = 'M';
            memoryBlock[memoryBlock.length - 7] = 'E';
            memoryBlock[memoryBlock.length - 6] = 'M';
            memoryBlock[memoryBlock.length - 5] = 'B';
            memoryBlock[memoryBlock.length - 4] = 'L';
            memoryBlock[memoryBlock.length - 3] = 'O';
            memoryBlock[memoryBlock.length - 2] = 'C';
            memoryBlock[memoryBlock.length - 1] = 'K';
        }
    }
    
    /**
     * 批量申请内存（用于快速触发OOM）
     */
    public BatchAllocationResult batchAllocateMemory(int sizeMB, int count) {
        List<MemoryAllocationResult> results = new ArrayList<>();
        int successCount = 0;
        int failureCount = 0;
        
        log.info("开始批量申请内存 - 每个块大小: {}MB, 数量: {}", sizeMB, count);
        
        for (int i = 0; i < count; i++) {
            String requestId = "batch-" + System.currentTimeMillis() + "-" + i;
            MemoryAllocationResult result = allocateMemory(requestId, sizeMB);
            results.add(result);
            
            if (result.isSuccess()) {
                successCount++;
            } else {
                failureCount++;
                log.warn("批量申请失败 - 第{}个, 请求ID: {}, 错误: {}", 
                        i + 1, requestId, result.getMessage());
                break; // 一旦失败就停止，避免无意义的尝试
            }
            
            // 短暂延迟，让系统有时间响应
            try {
                Thread.sleep(100);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            }
        }
        
        log.info("批量申请完成 - 成功: {}, 失败: {}", successCount, failureCount);
        
        return BatchAllocationResult.builder()
                .totalCount(count)
                .successCount(successCount)
                .failureCount(failureCount)
                .results(results)
                .build();
    }
    
    /**
     * 释放指定的内存块
     */
    public MemoryReleaseResult releaseMemory(String requestId) {
        MemoryBlock memoryBlock = memoryBlocks.remove(requestId);
        if (memoryBlock != null) {
            long sizeBytes = memoryBlock.data().length;
            totalMemoryUsed.addAndGet(-sizeBytes);
            long currentBlockCount = blockCount.decrementAndGet();
            
            log.info("内存释放成功 - 请求ID: {}, 大小: {}MB, 总内存: {}MB, 块数量: {}", 
                    requestId, sizeBytes / (1024 * 1024), totalMemoryUsed.get() / (1024 * 1024), currentBlockCount);
            
            return MemoryReleaseResult.builder()
                    .success(true)
                    .requestId(requestId)
                    .releasedSizeMB(sizeBytes / (1024 * 1024))
                    .totalMemoryUsedMB(totalMemoryUsed.get() / (1024 * 1024))
                    .blockCount(currentBlockCount)
                    .message("内存释放成功")
                    .build();
        } else {
            return MemoryReleaseResult.builder()
                    .success(false)
                    .requestId(requestId)
                    .message("未找到指定的内存块")
                    .build();
        }
    }
    
    /**
     * 获取内存使用统计信息
     */
    public MemoryStats getMemoryStats() {
        Runtime runtime = Runtime.getRuntime();
        long maxMemory = runtime.maxMemory();
        long totalMemory = runtime.totalMemory();
        long freeMemory = runtime.freeMemory();
        
        // 修复：使用我们应用层跟踪的内存使用量，而不是 JVM 堆内存使用量
        long usedMemory = totalMemoryUsed.get();  // 使用应用层跟踪的内存使用量
        long heapUsedMemory = totalMemory - freeMemory;  // JVM 堆内存使用量
        
        // 计算总内存使用量（常驻 + 可回收）
        long totalMemoryUsedMB = usedMemory + totalGcableMemoryUsed.get();
        
        return MemoryStats.builder()
                .maxMemoryMB(maxMemory / (1024 * 1024))
                .totalMemoryMB(totalMemory / (1024 * 1024))
                .freeMemoryMB(freeMemory / (1024 * 1024))
                .usedMemoryMB(usedMemory / (1024 * 1024))  // 修复：使用应用层内存使用量
                .allocatedBlocksMB(usedMemory / (1024 * 1024))  // 保持一致性
                .blockCount(blockCount.get())
                .gcableMemoryMB(totalGcableMemoryUsed.get() / (1024 * 1024))
                .gcableBlockCount(gcableBlockCount.get())
                .totalMemoryUsedMB(totalMemoryUsedMB / (1024 * 1024))
                .build();
    }
    
    /**
     * 清空所有内存块
     */
    public MemoryClearResult clearAllMemory() {
        long totalSize = totalMemoryUsed.get();
        long blockCountValue = blockCount.get();
        
        memoryBlocks.clear();
        totalMemoryUsed.set(0);
        blockCount.set(0);
        
        log.info("所有内存已清空 - 释放大小: {}MB, 块数量: {}", 
                totalSize / (1024 * 1024), blockCountValue);
        
        return MemoryClearResult.builder()
                .success(true)
                .clearedSizeMB(totalSize / (1024 * 1024))
                .clearedBlockCount(blockCountValue)
                .message("所有内存已清空")
                .build();
    }
    
    /**
     * 强制垃圾回收并返回结果
     */
    public GarbageCollectionResult forceGarbageCollection() {
        log.info("开始强制垃圾回收...");
        
        // 记录GC前的内存状态
        Runtime runtime = Runtime.getRuntime();
        long beforeTotalMemory = runtime.totalMemory();
        long beforeFreeMemory = runtime.freeMemory();
        long beforeUsedMemory = beforeTotalMemory - beforeFreeMemory;
        
        // 记录应用层内存状态
        long beforeResidentMemory = totalMemoryUsed.get();
        long beforeGcableMemory = totalGcableMemoryUsed.get();
        
        log.info("GC前 - JVM总内存: {}MB, 已用: {}MB, 空闲: {}MB, 常驻: {}MB, 可回收: {}MB", 
                beforeTotalMemory / (1024 * 1024), 
                beforeUsedMemory / (1024 * 1024), 
                beforeFreeMemory / (1024 * 1024),
                beforeResidentMemory / (1024 * 1024),
                beforeGcableMemory / (1024 * 1024));
        
        // 强制触发垃圾回收
        long startTime = System.currentTimeMillis();
        System.gc();
        long endTime = System.currentTimeMillis();
        
        // 等待GC完成
        try {
            Thread.sleep(1000);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        
        // 记录GC后的内存状态
        long afterTotalMemory = runtime.totalMemory();
        long afterFreeMemory = runtime.freeMemory();
        long afterUsedMemory = afterTotalMemory - afterFreeMemory;
        
        // 记录应用层内存状态
        long afterResidentMemory = totalMemoryUsed.get();
        long afterGcableMemory = totalGcableMemoryUsed.get();
        
        // 计算内存变化
        long memoryFreed = beforeUsedMemory - afterUsedMemory;
        long residentMemoryFreed = beforeResidentMemory - afterResidentMemory;
        long gcableMemoryFreed = beforeGcableMemory - afterGcableMemory;
        long gcDuration = endTime - startTime;
        
        log.info("GC后 - JVM总内存: {}MB, 已用: {}MB, 空闲: {}MB, 释放: {}MB, 常驻: {}MB, 可回收: {}MB, 耗时: {}ms", 
                afterTotalMemory / (1024 * 1024), 
                afterUsedMemory / (1024 * 1024), 
                afterFreeMemory / (1024 * 1024),
                memoryFreed / (1024 * 1024),
                afterResidentMemory / (1024 * 1024),
                afterGcableMemory / (1024 * 1024),
                gcDuration);
        
        log.info("GC效果分析 - 常驻内存释放: {}MB, 可回收内存释放: {}MB", 
                residentMemoryFreed / (1024 * 1024),
                gcableMemoryFreed / (1024 * 1024));
        
        return GarbageCollectionResult.builder()
                .success(true)
                .beforeUsedMemoryMB(beforeUsedMemory / (1024 * 1024))
                .afterUsedMemoryMB(afterUsedMemory / (1024 * 1024))
                .memoryFreedMB(memoryFreed / (1024 * 1024))
                .gcDurationMs(gcDuration)
                .totalMemoryMB(afterTotalMemory / (1024 * 1024))
                .freeMemoryMB(afterFreeMemory / (1024 * 1024))
                .message("垃圾回收完成")
                .build();
    }
    
    /**
     * 部分释放内存（释放指定数量的内存块）
     */
    public PartialReleaseResult partialReleaseMemory(int blockCountToRelease) {
        if (blockCountToRelease <= 0) {
            return PartialReleaseResult.builder()
                    .success(false)
                    .message("释放块数量必须大于0")
                    .build();
        }
        
        int currentBlockCount = (int) blockCount.get();
        if (blockCountToRelease > currentBlockCount) {
            blockCountToRelease = currentBlockCount;
        }
        
        log.info("开始部分释放内存 - 当前块数: {}, 要释放: {}", currentBlockCount, blockCountToRelease);
        
        long totalFreedSize = 0;
        int actualFreedCount = 0;
        
        // 获取要释放的内存块
        List<String> keysToRemove = new ArrayList<>();
        for (Map.Entry<String, MemoryBlock> entry : memoryBlocks.entrySet()) {
            if (actualFreedCount >= blockCountToRelease) {
                break;
            }
            keysToRemove.add(entry.getKey());
            totalFreedSize += entry.getValue().data().length;
            actualFreedCount++;
        }
        
        // 释放内存块
        for (String key : keysToRemove) {
            memoryBlocks.remove(key);
        }
        
        // 更新统计信息
        totalMemoryUsed.addAndGet(-totalFreedSize);
        blockCount.addAndGet(-actualFreedCount);
        
        log.info("部分释放完成 - 释放块数: {}, 释放大小: {}MB", 
                actualFreedCount, totalFreedSize / (1024 * 1024));
        
        return PartialReleaseResult.builder()
                .success(true)
                .requestedBlockCount(blockCountToRelease)
                .actualFreedBlockCount(actualFreedCount)
                .freedSizeMB(totalFreedSize / (1024 * 1024))
                .remainingBlockCount(blockCount.get())
                .remainingSizeMB(totalMemoryUsed.get() / (1024 * 1024))
                .message("部分释放完成")
                .build();
    }
    
    /**
     * 获取内存使用详情
     */
    public MemoryDetailResult getMemoryDetails() {
        Runtime runtime = Runtime.getRuntime();
        long maxMemory = runtime.maxMemory();
        long totalMemory = runtime.totalMemory();
        long freeMemory = runtime.freeMemory();
        
        // 修复：使用我们应用层跟踪的内存使用量，而不是 JVM 堆内存使用量
        long usedMemory = totalMemoryUsed.get();  // 使用应用层跟踪的内存使用量
        long heapUsedMemory = totalMemory - freeMemory;  // JVM 堆内存使用量
        
        // 计算内存使用率
        double heapUsagePercent = (double) heapUsedMemory / maxMemory * 100;
        double allocatedUsagePercent = (double) usedMemory / maxMemory * 100;
        
        // 获取内存块详情
        List<MemoryDetailResult.MemoryBlockInfo> blockInfos = new ArrayList<>();
        for (Map.Entry<String, MemoryBlock> entry : memoryBlocks.entrySet()) {
            MemoryBlock block = entry.getValue();
            blockInfos.add(MemoryDetailResult.MemoryBlockInfo.builder()
                    .requestId(block.requestId())
                    .sizeMB(block.sizeMB())
                    .timestamp(block.timestamp())
                    .ageMinutes((System.currentTimeMillis() - block.timestamp().toEpochMilli()) / (1000 * 60))
                    .build());
        }
        
        return MemoryDetailResult.builder()
                .maxMemoryMB(maxMemory / (1024 * 1024))
                .totalMemoryMB(totalMemory / (1024 * 1024))
                .freeMemoryMB(freeMemory / (1024 * 1024))
                .usedMemoryMB(usedMemory / (1024 * 1024))  // 修复：使用应用层内存使用量
                .allocatedBlocksMB(usedMemory / (1024 * 1024))  // 保持一致性
                .blockCount(blockCount.get())
                .heapUsagePercent(Math.round(heapUsagePercent * 100.0) / 100.0)
                .allocatedUsagePercent(Math.round(allocatedUsagePercent * 100.0) / 100.0)
                .memoryBlocks(blockInfos)
                .build();
    }
    
    /**
     * 获取内存对比分析结果
     */
    public MemoryComparisonResult getMemoryComparison() {
        Runtime runtime = Runtime.getRuntime();
        long maxMemory = runtime.maxMemory();
        
        // 获取当前内存状态
        long residentMemory = totalMemoryUsed.get();
        long gcableMemory = totalGcableMemoryUsed.get();
        long totalMemory = residentMemory + gcableMemory;
        
        // 计算占比
        double residentPercent = totalMemory > 0 ? (double) residentMemory / totalMemory * 100 : 0;
        double gcablePercent = totalMemory > 0 ? (double) gcableMemory / totalMemory * 100 : 0;
        
        // 评估内存管理效率
        String efficiency;
        String recommendation;
        
        if (gcablePercent > 70) {
            efficiency = "高";
            recommendation = "可回收内存比例较高，GC效果良好，内存管理效率高";
        } else if (gcablePercent > 30) {
            efficiency = "中";
            recommendation = "可回收内存比例适中，GC效果一般，建议优化内存使用策略";
        } else {
            efficiency = "低";
            recommendation = "可回收内存比例较低，GC效果差，建议减少常驻内存使用";
        }
        
        // 构建常驻内存统计
        MemoryComparisonResult.ResidentMemoryStats residentStats = MemoryComparisonResult.ResidentMemoryStats.builder()
                .usedMemoryMB(residentMemory / (1024 * 1024))
                .blockCount(blockCount.get())
                .beforeGCMB(residentMemory / (1024 * 1024))
                .afterGCMB(residentMemory / (1024 * 1024))  // 常驻内存GC无效
                .gcEffectMB(0)  // 常驻内存GC无效
                .build();
        
        // 构建可回收内存统计
        MemoryComparisonResult.GcableMemoryStats gcableStats = MemoryComparisonResult.GcableMemoryStats.builder()
                .usedMemoryMB(gcableMemory / (1024 * 1024))
                .blockCount(gcableBlockCount.get())
                .beforeGCMB(gcableMemory / (1024 * 1024))
                .afterGCMB(0)  // 可回收内存GC后应该为0
                .gcEffectMB(gcableMemory / (1024 * 1024))  // 可回收内存GC有效
                .build();
        
        // 构建对比分析
        MemoryComparisonResult.ComparisonAnalysis analysis = MemoryComparisonResult.ComparisonAnalysis.builder()
                .totalMemoryMB(totalMemory / (1024 * 1024))
                .residentMemoryPercent(Math.round(residentPercent * 100.0) / 100.0)
                .gcableMemoryPercent(Math.round(gcablePercent * 100.0) / 100.0)
                .totalGCEffectMB(gcableMemory / (1024 * 1024))  // 只有可回收内存能被GC回收
                .memoryEfficiency(efficiency)
                .recommendation(recommendation)
                .build();
        
        return MemoryComparisonResult.builder()
                .residentMemory(residentStats)
                .gcableMemory(gcableStats)
                .analysis(analysis)
                .build();
    }
}
