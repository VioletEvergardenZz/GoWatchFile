package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "内存清理结果")
public class MemoryClearResult {
    @Schema(description = "是否成功", example = "true")
    private boolean success;
    
    @Schema(description = "清理的内存大小(MB)", example = "500")
    private long clearedSizeMB;
    
    @Schema(description = "清理的内存块数量", example = "5")
    private long clearedBlockCount;
    
    @Schema(description = "消息", example = "内存清理完成")
    private String message;
}
