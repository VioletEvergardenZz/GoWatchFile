// 本文件用于固化知识库阶段门禁阈值，避免前后端和脚本口径漂移

package kb

const (
	// StageGateSearchHitRatioMin 表示检索命中率最低门禁
	StageGateSearchHitRatioMin = 0.70
	// StageGateAskCitationRatioMin 表示问答引用率最低门禁
	StageGateAskCitationRatioMin = 0.95
	// StageGateReviewLatencyP95MsMax 表示评审时延 P95 最大门禁（毫秒）
	StageGateReviewLatencyP95MsMax = 800
)

// QualityGates 用于对外统一暴露知识库阶段门禁阈值
type QualityGates struct {
	SearchHitRatioMin     float64 `json:"searchHitRatioMin"`
	AskCitationRatioMin   float64 `json:"askCitationRatioMin"`
	ReviewLatencyP95MsMax int     `json:"reviewLatencyP95MsMax"`
}

// DefaultQualityGates 返回当前阶段固定门禁阈值
func DefaultQualityGates() QualityGates {
	return QualityGates{
		SearchHitRatioMin:     StageGateSearchHitRatioMin,
		AskCitationRatioMin:   StageGateAskCitationRatioMin,
		ReviewLatencyP95MsMax: StageGateReviewLatencyP95MsMax,
	}
}
