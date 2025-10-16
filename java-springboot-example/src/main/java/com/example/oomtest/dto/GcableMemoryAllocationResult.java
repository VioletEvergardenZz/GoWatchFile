package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "可回收内存申请结果")
public class GcableMemoryAllocationResult {
    @Schema(description = "是否成功", example = "true")
    private boolean success;
    
    @Schema(description = "请求ID", example = "gcable_001")
    private String requestId;
    
    @Schema(description = "申请的内存大小(MB)", example = "100")
    private int allocatedSizeMB;
    
    @Schema(description = "当前可回收内存总量(MB)", example = "500")
    private long totalGcableMemoryMB;
    
    @Schema(description = "当前常驻内存总量(MB)", example = "200")
    private long totalResidentMemoryMB;
    
    @Schema(description = "可回收内存块数量", example = "5")
    private long gcableBlockCount;
    
    @Schema(description = "常驻内存块数量", example = "2")
    private long residentBlockCount;
    
    @Schema(description = "消息", example = "可回收内存申请成功")
    private String message;
    
    @Schema(description = "是否已释放引用", example = "true")
    private boolean referenceReleased;
}
