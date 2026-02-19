package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type controlTaskFailureReasonDTO struct {
	Reason        string         `json:"reason"`
	Count         int            `json:"count"`
	Statuses      map[string]int `json:"statuses,omitempty"`
	SampleTaskIDs []string       `json:"sampleTaskIds,omitempty"`
}

type controlTaskFailureReasonAgg struct {
	reason        string
	count         int
	statuses      map[string]int
	sampleTaskIDs []string
}

func (h *handler) controlTaskFailureReasonsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	statuses, err := parseControlFailureStatuses(r.URL.Query().Get("status"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	limit := parseControlListLimit(r.URL.Query().Get("limit"), 10, 100)

	h.controlMu.RLock()
	defer h.controlMu.RUnlock()

	buckets := map[string]*controlTaskFailureReasonAgg{}
	for _, task := range h.controlTasks {
		if !statuses[task.Status] {
			continue
		}
		if typeFilter != "" && task.Type != typeFilter {
			continue
		}
		reason := strings.TrimSpace(task.FailureReason)
		if reason == "" {
			reason = defaultControlTaskFailureReason(task.Status)
		}
		key := strings.ToLower(reason)
		item, ok := buckets[key]
		if !ok {
			item = &controlTaskFailureReasonAgg{
				reason:   reason,
				statuses: map[string]int{},
			}
			buckets[key] = item
		}
		item.count++
		item.statuses[task.Status]++
		if len(item.sampleTaskIDs) < 3 && strings.TrimSpace(task.ID) != "" {
			item.sampleTaskIDs = append(item.sampleTaskIDs, task.ID)
		}
	}

	items := make([]controlTaskFailureReasonDTO, 0, len(buckets))
	for _, item := range buckets {
		items = append(items, controlTaskFailureReasonDTO{
			Reason:        item.reason,
			Count:         item.count,
			Statuses:      item.statuses,
			SampleTaskIDs: item.sampleTaskIDs,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Reason < items[j].Reason
	})

	total := len(items)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
		"total": total,
		"filters": map[string]any{
			"status": sortedControlFailureStatuses(statuses),
			"type":   typeFilter,
			"limit":  limit,
		},
	})
}

func parseControlFailureStatuses(raw string) (map[string]bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]bool{
			controlTaskStatusFailed:  true,
			controlTaskStatusTimeout: true,
		}, nil
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
	out := map[string]bool{}
	for _, part := range parts {
		status := strings.ToLower(strings.TrimSpace(part))
		switch status {
		case controlTaskStatusFailed, controlTaskStatusTimeout, controlTaskStatusCanceled:
			out[status] = true
		case "":
			continue
		default:
			return nil, fmt.Errorf("invalid status filter: %s", status)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("status filter is empty")
	}
	return out, nil
}

func sortedControlFailureStatuses(statuses map[string]bool) []string {
	if len(statuses) == 0 {
		return nil
	}
	out := make([]string, 0, len(statuses))
	for status := range statuses {
		out = append(out, status)
	}
	sort.Strings(out)
	return out
}

func pickControlTaskFailureReason(errorText, message string) string {
	if trimmed := strings.TrimSpace(errorText); trimmed != "" {
		return trimmed
	}
	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		return "task_failed"
	}
	lower := strings.ToLower(trimmedMessage)
	if strings.HasPrefix(lower, "error=") {
		value := strings.TrimSpace(trimmedMessage[len("error="):])
		if value != "" {
			return value
		}
	}
	return trimmedMessage
}

func defaultControlTaskFailureReason(status string) string {
	switch strings.TrimSpace(status) {
	case controlTaskStatusTimeout:
		return "run_timeout"
	case controlTaskStatusCanceled:
		return "manual_cancel"
	case controlTaskStatusFailed:
		return "task_failed"
	default:
		return "unknown"
	}
}
