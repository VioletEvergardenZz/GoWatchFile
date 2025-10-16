package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "批量内存申请请求")
public class BatchAllocationRequest {
    @Schema(description = "每个内存块大小(MB)", example = "100", minimum = "1", maximum = "1000")
    private int sizeMB;
    
    @Schema(description = "内存块数量", example = "10", minimum = "1", maximum = "100")
    private int count;
}
