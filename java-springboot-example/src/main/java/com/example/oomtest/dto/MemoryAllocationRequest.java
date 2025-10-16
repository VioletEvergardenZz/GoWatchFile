package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "内存申请请求")
public class MemoryAllocationRequest {
    @Schema(description = "请求ID", example = "mem_001", requiredMode = Schema.RequiredMode.NOT_REQUIRED)
    private String requestId;
    
    @Schema(description = "内存大小(MB)", example = "100", minimum = "1", maximum = "1000")
    private int sizeMB;
}
