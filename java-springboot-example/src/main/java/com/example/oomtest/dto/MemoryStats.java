package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "内存使用统计信息")
public class MemoryStats {
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
    
    @Schema(description = "可回收内存总量(MB)", example = "300")
    private long gcableMemoryMB;
    
    @Schema(description = "可回收内存块数量", example = "3")
    private long gcableBlockCount;
    
    @Schema(description = "总内存使用量(MB)", example = "800")
    private long totalMemoryUsedMB;
}
