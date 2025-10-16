package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "内存对比分析结果")
public class MemoryComparisonResult {
    @Schema(description = "常驻内存统计")
    private ResidentMemoryStats residentMemory;
    
    @Schema(description = "可回收内存统计")
    private GcableMemoryStats gcableMemory;
    
    @Schema(description = "综合对比分析")
    private ComparisonAnalysis analysis;
    
    @Data
    @Builder
    @Schema(description = "常驻内存统计")
    public static class ResidentMemoryStats {
        @Schema(description = "内存使用量(MB)", example = "500")
        private long usedMemoryMB;
        
        @Schema(description = "内存块数量", example = "5")
        private long blockCount;
        
        @Schema(description = "GC 前内存(MB)", example = "500")
        private long beforeGCMB;
        
        @Schema(description = "GC 后内存(MB)", example = "500")
        private long afterGCMB;
        
        @Schema(description = "GC 效果(MB)", example = "0")
        private long gcEffectMB;
    }
    
    @Data
    @Builder
    @Schema(description = "可回收内存统计")
    public static class GcableMemoryStats {
        @Schema(description = "内存使用量(MB)", example = "300")
        private long usedMemoryMB;
        
        @Schema(description = "内存块数量", example = "3")
        private long blockCount;
        
        @Schema(description = "GC 前内存(MB)", example = "300")
        private long beforeGCMB;
        
        @Schema(description = "GC 后内存(MB)", example = "0")
        private long afterGCMB;
        
        @Schema(description = "GC 效果(MB)", example = "300")
        private long gcEffectMB;
    }
    
    @Data
    @Builder
    @Schema(description = "对比分析")
    public static class ComparisonAnalysis {
        @Schema(description = "总内存使用量(MB)", example = "800")
        private long totalMemoryMB;
        
        @Schema(description = "常驻内存占比(%)", example = "62.5")
        private double residentMemoryPercent;
        
        @Schema(description = "可回收内存占比(%)", example = "37.5")
        private double gcableMemoryPercent;
        
        @Schema(description = "GC 总体效果(MB)", example = "300")
        private long totalGCEffectMB;
        
        @Schema(description = "内存管理效率", example = "高")
        private String memoryEfficiency;
        
        @Schema(description = "建议", example = "可回收内存比例较高，GC效果良好")
        private String recommendation;
    }
}
