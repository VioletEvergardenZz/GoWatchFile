package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "内存释放结果")
public class MemoryReleaseResult {
    @Schema(description = "是否成功", example = "true")
    private boolean success;
    
    @Schema(description = "请求ID", example = "mem_001")
    private String requestId;
    
    @Schema(description = "释放的内存大小(MB)", example = "100")
    private long releasedSizeMB;
    
    @Schema(description = "剩余内存使用量(MB)", example = "400")
    private long totalMemoryUsedMB;
    
    @Schema(description = "剩余内存块数量", example = "4")
    private long blockCount;
    
    @Schema(description = "消息", example = "内存释放成功")
    private String message;
}
