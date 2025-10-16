package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;
import java.time.Instant;
import java.util.List;

@Data
@Builder
@Schema(description = "内存使用详情")
public class MemoryDetailResult {
    @Schema(description = "JVM 最大内存(MB)", example = "2048")
    private long maxMemoryMB;
    
    @Schema(description = "JVM 已分配内存(MB)", example = "1024")
    private long totalMemoryMB;
    
    @Schema(description = "JVM 空闲内存(MB)", example = "512")
    private long freeMemoryMB;
    
    @Schema(description = "JVM 已使用内存(MB)", example = "512")
    private long usedMemoryMB;
    
    @Schema(description = "常驻内存块总量(MB)", example = "500")
    private long allocatedBlocksMB;
    
    @Schema(description = "常驻内存块数量", example = "5")
    private long blockCount;
    
    @Schema(description = "堆内存使用率(%)", example = "25.0")
    private double heapUsagePercent;
    
    @Schema(description = "常驻内存使用率(%)", example = "24.4")
    private double allocatedUsagePercent;
    
    @Schema(description = "内存块详情列表")
    private List<MemoryBlockInfo> memoryBlocks;
    
    @Data
    @Builder
    @Schema(description = "内存块信息")
    public static class MemoryBlockInfo {
        @Schema(description = "请求ID", example = "mem_001")
        private String requestId;
        
        @Schema(description = "内存块大小(MB)", example = "100")
        private int sizeMB;
        
        @Schema(description = "申请时间", example = "2024-01-01T12:00:00Z")
        private Instant timestamp;
        
        @Schema(description = "存活时间(分钟)", example = "30")
        private long ageMinutes;
    }
}
