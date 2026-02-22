package api

import (
	"strings"

	"file-watch/internal/alert"
)

// currentAlertState 统一封装告警状态读取路径
// 运行态走 FileService，测试场景允许用 override 注入，避免构造完整服务依赖
func (h *handler) currentAlertState() *alert.State {
	if h == nil {
		return nil
	}
	if h.alertStateOverride != nil {
		return h.alertStateOverride
	}
	if h.fs == nil {
		return nil
	}
	return h.fs.AlertState()
}

func (h *handler) findAlertDecision(alertID string) (alert.Decision, bool) {
	state := h.currentAlertState()
	if state == nil {
		return alert.Decision{}, false
	}
	trimmedID := strings.TrimSpace(alertID)
	if trimmedID == "" {
		return alert.Decision{}, false
	}
	return state.GetDecision(trimmedID)
}
