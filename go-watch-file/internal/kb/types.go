// 本文件用于知识库领域类型定义 统一约束条目与操作输入输出结构

// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package kb

import "errors"

const (
	StatusDraft     = "draft"
	StatusReviewing = "reviewing"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

const (
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"
)

var (
	ErrNotFound     = errors.New("knowledge article not found")
	ErrInvalidInput = errors.New("invalid knowledge input")
)

type Article struct {
	ID             string           `json:"id"`
	Title          string           `json:"title"`
	Summary        string           `json:"summary"`
	Category       string           `json:"category"`
	Severity       string           `json:"severity"`
	Status         string           `json:"status"`
	NeedsReview    bool             `json:"needsReview"`
	CurrentVersion int              `json:"currentVersion"`
	Content        string           `json:"content,omitempty"`
	ChangeNote     string           `json:"changeNote,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
	References     []ArticleRef     `json:"references,omitempty"`
	Versions       []ArticleVersion `json:"versions,omitempty"`
	Reviews        []ReviewRecord   `json:"reviews,omitempty"`
	CreatedBy      string           `json:"createdBy"`
	UpdatedBy      string           `json:"updatedBy"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
}

type ArticleVersion struct {
	Version    int    `json:"version"`
	Content    string `json:"content"`
	ChangeNote string `json:"changeNote"`
	SourceType string `json:"sourceType"`
	SourceRef  string `json:"sourceRef,omitempty"`
	CreatedBy  string `json:"createdBy"`
	CreatedAt  string `json:"createdAt"`
}

type ReviewRecord struct {
	Action    string `json:"action"`
	Comment   string `json:"comment,omitempty"`
	Operator  string `json:"operator"`
	CreatedAt string `json:"createdAt"`
}

type ArticleRef struct {
	RefType  string `json:"refType"`
	RefPath  string `json:"refPath"`
	RefTitle string `json:"refTitle"`
}

type ListQuery struct {
	Query           string
	Status          string
	Severity        string
	Tag             string
	Page            int
	PageSize        int
	IncludeArchived bool
}

type CreateArticleInput struct {
	Title      string
	Summary    string
	Category   string
	Severity   string
	Content    string
	Tags       []string
	CreatedBy  string
	ChangeNote string
	SourceType string
	SourceRef  string
	RefTitle   string
}

type UpdateArticleInput struct {
	Title      string
	Summary    string
	Category   string
	Severity   string
	Content    string
	Tags       []string
	UpdatedBy  string
	ChangeNote string
	SourceType string
	SourceRef  string
	RefTitle   string
}

type AskResult struct {
	Answer     string     `json:"answer"`
	Citations  []Citation `json:"citations"`
	Confidence float64    `json:"confidence"`
}

type Citation struct {
	ArticleID string `json:"articleId"`
	Title     string `json:"title"`
	Version   int    `json:"version"`
}

type ImportResult struct {
	Imported int      `json:"imported"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Files    []string `json:"files,omitempty"`
}
