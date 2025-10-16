package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "内存申请结果")
public class MemoryAllocationResult {
    @Schema(description = "是否成功", example = "true")
    private boolean success;
    
    @Schema(description = "请求ID", example = "mem_001")
    private String requestId;
    
    @Schema(description = "申请的内存大小(MB)", example = "100")
    private int allocatedSizeMB;
    
    @Schema(description = "总内存使用量(MB)", example = "500")
    private long totalMemoryUsedMB;
    
    @Schema(description = "内存块数量", example = "5")
    private long blockCount;
    
    @Schema(description = "申请次数", example = "10")
    private int allocationCount;
    
    @Schema(description = "消息", example = "内存申请成功")
    private String message;
}
