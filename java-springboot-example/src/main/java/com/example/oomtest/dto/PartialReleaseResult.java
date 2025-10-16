package com.example.oomtest.dto;

import io.swagger.v3.oas.annotations.media.Schema;
import lombok.Builder;
import lombok.Data;

@Data
@Builder
@Schema(description = "部分内存释放结果")
public class PartialReleaseResult {
    @Schema(description = "是否成功", example = "true")
    private boolean success;
    
    @Schema(description = "请求释放的块数量", example = "3")
    private int requestedBlockCount;
    
    @Schema(description = "实际释放的块数量", example = "3")
    private int actualFreedBlockCount;
    
    @Schema(description = "释放的内存大小(MB)", example = "300")
    private long freedSizeMB;
    
    @Schema(description = "剩余块数量", example = "2")
    private long remainingBlockCount;
    
    @Schema(description = "剩余内存大小(MB)", example = "200")
    private long remainingSizeMB;
    
    @Schema(description = "消息", example = "部分释放完成")
    private String message;
}
