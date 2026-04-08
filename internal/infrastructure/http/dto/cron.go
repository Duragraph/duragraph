package dto

import "time"

// CreateCronRequest represents the request to create a cron job.
type CreateCronRequest struct {
	AssistantID       string                 `json:"assistant_id"`
	Schedule          string                 `json:"schedule"`
	Input             map[string]interface{} `json:"input,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Config            map[string]interface{} `json:"config,omitempty"`
	Context           map[string]interface{} `json:"context,omitempty"`
	Webhook           string                 `json:"webhook,omitempty"`
	MultitaskStrategy string                 `json:"multitask_strategy,omitempty"`
	OnRunCompleted    string                 `json:"on_run_completed,omitempty"`
	EndTime           *time.Time             `json:"end_time,omitempty"`
	Enabled           *bool                  `json:"enabled,omitempty"`
	Timezone          string                 `json:"timezone,omitempty"`
	InterruptBefore   []string               `json:"interrupt_before,omitempty"`
	InterruptAfter    []string               `json:"interrupt_after,omitempty"`
}

// UpdateCronRequest represents the request to update a cron job.
type UpdateCronRequest struct {
	Schedule        *string                `json:"schedule,omitempty"`
	Input           map[string]interface{} `json:"input,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Config          map[string]interface{} `json:"config,omitempty"`
	Context         map[string]interface{} `json:"context,omitempty"`
	Webhook         *string                `json:"webhook,omitempty"`
	OnRunCompleted  *string                `json:"on_run_completed,omitempty"`
	EndTime         *time.Time             `json:"end_time,omitempty"`
	Enabled         *bool                  `json:"enabled,omitempty"`
	Timezone        *string                `json:"timezone,omitempty"`
	InterruptBefore []string               `json:"interrupt_before,omitempty"`
	InterruptAfter  []string               `json:"interrupt_after,omitempty"`
}

// SearchCronsRequest represents the request to search cron jobs.
type SearchCronsRequest struct {
	AssistantID *string `json:"assistant_id,omitempty"`
	ThreadID    *string `json:"thread_id,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	Offset      int     `json:"offset,omitempty"`
	SortBy      string  `json:"sort_by,omitempty"`
	SortOrder   string  `json:"sort_order,omitempty"`
}

// CountCronsRequest represents the request to count cron jobs.
type CountCronsRequest struct {
	AssistantID *string `json:"assistant_id,omitempty"`
	ThreadID    *string `json:"thread_id,omitempty"`
}

// CronResponse represents a cron job in API responses.
type CronResponse struct {
	CronID         string                 `json:"cron_id"`
	AssistantID    string                 `json:"assistant_id"`
	ThreadID       *string                `json:"thread_id,omitempty"`
	Schedule       string                 `json:"schedule"`
	Timezone       string                 `json:"timezone"`
	Payload        map[string]interface{} `json:"payload"`
	Metadata       map[string]interface{} `json:"metadata"`
	Enabled        bool                   `json:"enabled"`
	OnRunCompleted string                 `json:"on_run_completed"`
	EndTime        *time.Time             `json:"end_time,omitempty"`
	NextRunDate    *time.Time             `json:"next_run_date,omitempty"`
	UserID         *string                `json:"user_id,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}
