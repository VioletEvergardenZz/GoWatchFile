package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;
import java.util.List;

@Data
@Builder
@Schema(description = "批量内存申请结果")
public class BatchAllocationResult {
    @Schema(description = "总申请数量", example = "10")
    private int totalCount;
    
    @Schema(description = "成功数量", example = "8")
    private int successCount;
    
    @Schema(description = "失败数量", example = "2")
    private int failureCount;
    
    @Schema(description = "申请结果列表")
    private List<MemoryAllocationResult> results;
    
    @Schema(description = "消息", example = "批量申请完成")
    private String message;
}
