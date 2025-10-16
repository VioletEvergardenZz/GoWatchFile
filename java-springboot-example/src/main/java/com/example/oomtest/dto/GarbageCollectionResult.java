package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "垃圾回收结果")
public class GarbageCollectionResult {
    @Schema(description = "是否成功", example = "true")
    private boolean success;
    
    @Schema(description = "GC前内存使用量(MB)", example = "800")
    private long beforeUsedMemoryMB;
    
    @Schema(description = "GC后内存使用量(MB)", example = "600")
    private long afterUsedMemoryMB;
    
    @Schema(description = "释放的内存(MB)", example = "200")
    private long memoryFreedMB;
    
    @Schema(description = "GC耗时(毫秒)", example = "150")
    private long gcDurationMs;
    
    @Schema(description = "总内存(MB)", example = "1024")
    private long totalMemoryMB;
    
    @Schema(description = "空闲内存(MB)", example = "424")
    private long freeMemoryMB;
    
    @Schema(description = "消息", example = "垃圾回收完成")
    private String message;
}
