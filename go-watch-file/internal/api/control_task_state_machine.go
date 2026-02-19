package api

import "time"

func isControlTaskRetryableStatus(status string) bool {
	return status == controlTaskStatusFailed || status == controlTaskStatusTimeout || status == controlTaskStatusCanceled
}

func isControlTaskCancelableStatus(status string) bool {
	return status == controlTaskStatusPending || status == controlTaskStatusAssigned || status == controlTaskStatusRunning
}

func hasControlTaskRetryBudget(state controlTaskState) bool {
	return state.RetryCount < state.MaxRetries
}

func buildControlTaskRetriedState(state controlTaskState, now time.Time) controlTaskState {
	next := state
	next.RetryCount++
	next.Status = controlTaskStatusPending
	next.FailureReason = ""
	next.AssignedAgentID = ""
	next.UpdatedAt = now
	next.FinishedAt = nil
	return next
}
