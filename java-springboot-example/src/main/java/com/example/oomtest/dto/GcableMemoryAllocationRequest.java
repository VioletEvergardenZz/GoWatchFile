package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "可回收内存申请请求")
public class GcableMemoryAllocationRequest {
    @Schema(description = "请求ID", example = "gcable_001", requiredMode = Schema.RequiredMode.NOT_REQUIRED)
    private String requestId;
    
    @Schema(description = "内存大小(MB)", example = "100", minimum = "1", maximum = "1000")
    private int sizeMB;
    
    @Schema(description = "是否立即释放引用", example = "true")
    private boolean releaseImmediately;
}
