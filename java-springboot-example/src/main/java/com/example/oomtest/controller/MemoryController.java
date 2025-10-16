package com.example.oomtest.controller;

import com.example.oomtest.dto.*;
import com.example.oomtest.service.MemoryService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import io.swagger.v3.oas.annotations.responses.ApiResponse;
import io.swagger.v3.oas.annotations.responses.ApiResponses;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.UUID;

@RestController
@RequestMapping("/api/memory")
@Tag(name = "内存管理", description = "内存申请、释放、监控等操作")
@Slf4j
public class MemoryController {
    
    @Autowired
    private MemoryService memoryService;
    
    /**
     * 申请内存
     */
    @Operation(summary = "申请常驻内存", description = "申请指定大小的内存块，常驻在堆中不会被GC回收")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "申请成功"),
        @ApiResponse(responseCode = "400", description = "请求参数错误"),
        @ApiResponse(responseCode = "500", description = "服务器内部错误")
    })
    @PostMapping("/allocate")
    public MemoryAllocationResult allocateMemory(@RequestBody MemoryAllocationRequest request) {
        // 如果没有提供请求ID，自动生成一个
        if (request.getRequestId() == null || request.getRequestId().trim().isEmpty()) {
            request.setRequestId(UUID.randomUUID().toString());
        }
        
        log.info("收到常驻内存申请请求 - ID: {}, 大小: {}MB", request.getRequestId(), request.getSizeMB());
        
        return memoryService.allocateMemory(request.getRequestId(), request.getSizeMB());
    }
    
    /**
     * 申请可回收内存
     */
    @Operation(summary = "申请可回收内存", description = "申请指定大小的内存块，可以被GC回收")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "申请成功"),
        @ApiResponse(responseCode = "400", description = "请求参数错误"),
        @ApiResponse(responseCode = "500", description = "服务器内部错误")
    })
    @PostMapping("/allocate-gcable")
    public GcableMemoryAllocationResult allocateGcableMemory(@RequestBody GcableMemoryAllocationRequest request) {
        // 如果没有提供请求ID，自动生成一个
        if (request.getRequestId() == null || request.getRequestId().trim().isEmpty()) {
            request.setRequestId(UUID.randomUUID().toString());
        }
        
        log.info("收到可回收内存申请请求 - ID: {}, 大小: {}MB, 立即释放: {}", 
                request.getRequestId(), request.getSizeMB(), request.isReleaseImmediately());
        
        return memoryService.allocateGcableMemory(request.getRequestId(), request.getSizeMB(), request.isReleaseImmediately());
    }
    
    /**
     * 申请可回收内存（GET方式）
     */
    @Operation(summary = "申请可回收内存(GET)", description = "通过GET方式申请可回收内存块")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "申请成功"),
        @ApiResponse(responseCode = "400", description = "请求参数错误")
    })
    @GetMapping("/allocate-gcable")
    public GcableMemoryAllocationResult allocateGcableMemoryGet(
            @Parameter(description = "内存大小(MB)", example = "100") @RequestParam int sizeMB,
            @Parameter(description = "是否立即释放引用", example = "true") @RequestParam(defaultValue = "true") boolean releaseImmediately) {
        String requestId = UUID.randomUUID().toString();
        log.info("收到可回收内存申请请求(GET) - ID: {}, 大小: {}MB, 立即释放: {}", requestId, sizeMB, releaseImmediately);
        
        return memoryService.allocateGcableMemory(requestId, sizeMB, releaseImmediately);
    }
    
    /**
     * 申请常驻内存（GET方式，方便测试）
     */
    @Operation(summary = "申请常驻内存(GET)", description = "通过GET方式申请指定大小的常驻内存块")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "申请成功"),
        @ApiResponse(responseCode = "400", description = "请求参数错误")
    })
    @GetMapping("/allocate")
    public MemoryAllocationResult allocateMemoryGet(@Parameter(description = "内存大小(MB)", example = "100") @RequestParam int sizeMB) {
        String requestId = UUID.randomUUID().toString();
        log.info("收到常驻内存申请请求(GET) - ID: {}, 大小: {}MB", requestId, sizeMB);
        
        return memoryService.allocateMemory(requestId, sizeMB);
    }
    
    /**
     * 获取内存对比分析
     */
    @Operation(summary = "内存对比分析", description = "获取常驻内存和可回收内存的对比分析结果")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "获取成功")
    })
    @GetMapping("/stats-compare")
    public MemoryComparisonResult getMemoryComparison() {
        log.info("收到内存对比分析请求");
        return memoryService.getMemoryComparison();
    }
    
    /**
     * 批量申请内存（用于快速触发OOM）
     */
    @Operation(summary = "批量申请内存", description = "批量申请多个内存块，用于快速触发OOM")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "批量申请成功"),
        @ApiResponse(responseCode = "400", description = "请求参数错误"),
        @ApiResponse(responseCode = "500", description = "服务器内部错误")
    })
    @PostMapping("/batch-allocate")
    public BatchAllocationResult batchAllocateMemory(@RequestBody BatchAllocationRequest request) {
        log.info("收到批量内存申请请求 - 每个块大小: {}MB, 数量: {}", request.getSizeMB(), request.getCount());
        return memoryService.batchAllocateMemory(request.getSizeMB(), request.getCount());
    }
    
    /**
     * 批量申请内存（GET方式）
     */
    @Operation(summary = "批量申请内存(GET)", description = "通过GET方式批量申请内存块")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "批量申请成功"),
        @ApiResponse(responseCode = "400", description = "请求参数错误")
    })
    @GetMapping("/batch-allocate")
    public BatchAllocationResult batchAllocateMemoryGet(
            @Parameter(description = "每个内存块大小(MB)", example = "100") @RequestParam int sizeMB,
            @Parameter(description = "内存块数量", example = "10") @RequestParam int count) {
        log.info("收到批量内存申请请求(GET) - 每个块大小: {}MB, 数量: {}", sizeMB, count);
        return memoryService.batchAllocateMemory(sizeMB, count);
    }
    
    /**
     * 释放内存
     */
    @DeleteMapping("/release/{requestId}")
    public MemoryReleaseResult releaseMemory(@PathVariable String requestId) {
        log.info("收到内存释放请求 - ID: {}", requestId);
        return memoryService.releaseMemory(requestId);
    }
    
    /**
     * 获取内存统计信息
     */
    @Operation(summary = "获取内存统计", description = "获取当前内存使用统计信息")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "获取成功")
    })
    @GetMapping("/stats")
    public MemoryStats getMemoryStats() {
        return memoryService.getMemoryStats();
    }
    
    /**
     * 清空所有内存
     */
    @DeleteMapping("/clear")
    public MemoryClearResult clearAllMemory() {
        log.info("收到清空所有内存请求");
        return memoryService.clearAllMemory();
    }
    
    /**
     * 强制垃圾回收
     */
    @Operation(summary = "强制垃圾回收", description = "触发JVM垃圾回收，清理内存")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "垃圾回收完成")
    })
    @PostMapping("/gc")
    public GarbageCollectionResult forceGarbageCollection() {
        log.info("收到强制垃圾回收请求");
        return memoryService.forceGarbageCollection();
    }
    
    /**
     * 部分释放内存
     */
    @DeleteMapping("/partial-release")
    public PartialReleaseResult partialReleaseMemory(@RequestParam int blockCount) {
        log.info("收到部分释放内存请求 - 块数: {}", blockCount);
        return memoryService.partialReleaseMemory(blockCount);
    }
    
    /**
     * 获取内存使用详情
     */
    @GetMapping("/details")
    public MemoryDetailResult getMemoryDetails() {
        return memoryService.getMemoryDetails();
    }
    
    /**
     * 快速OOM测试接口
     */
    @Operation(summary = "OOM测试", description = "快速触发OutOfMemoryError测试，生成堆转储文件")
    @ApiResponses(value = {
        @ApiResponse(responseCode = "200", description = "OOM测试完成"),
        @ApiResponse(responseCode = "500", description = "测试过程中发生错误")
    })
    @GetMapping("/test-oom")
    public String testOOM() {
        log.info("开始OOM测试...");
        
        // 快速申请大量内存，触发OOM
        int blockSize = 100; // 100MB per block
        int blockCount = 50; // 50 blocks
        
        log.info("申请 {} 个 {}MB 内存块，总计 {}MB", blockCount, blockSize, blockSize * blockCount);
        
        try {
            BatchAllocationResult result = memoryService.batchAllocateMemory(blockSize, blockCount);
            return String.format("OOM测试完成 - 成功: %d, 失败: %d", 
                    result.getSuccessCount(), result.getFailureCount());
        } catch (Exception e) {
            return "OOM测试异常: " + e.getMessage();
        }
    }
    
    /**
     * 健康检查接口
     */
    @GetMapping("/health")
    public String health() {
        return "OK";
    }
}
