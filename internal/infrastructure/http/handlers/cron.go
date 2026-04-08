package handlers

import (
	"net/http"
	"time"

	"github.com/duragraph/duragraph/internal/application/service"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	"github.com/labstack/echo/v4"
)

// CronHandler handles LangGraph-compatible Crons API endpoints.
type CronHandler struct {
	repo *postgres.CronRepository
}

func NewCronHandler(repo *postgres.CronRepository) *CronHandler {
	return &CronHandler{repo: repo}
}

func cronToResponse(c *postgres.CronJob) dto.CronResponse {
	return dto.CronResponse{
		CronID:         c.CronID,
		AssistantID:    c.AssistantID,
		ThreadID:       c.ThreadID,
		Schedule:       c.Schedule,
		Timezone:       c.Timezone,
		Payload:        c.Payload,
		Metadata:       c.Metadata,
		Enabled:        c.Enabled,
		OnRunCompleted: c.OnRunCompleted,
		EndTime:        c.EndTime,
		NextRunDate:    c.NextRunDate,
		UserID:         c.UserID,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

func (h *CronHandler) createCron(c echo.Context, threadID *string) error {
	var req dto.CreateCronRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	if req.AssistantID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: "assistant_id is required",
		})
	}
	if req.Schedule == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: "schedule is required",
		})
	}
	if err := service.ValidateSchedule(req.Schedule); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	onRunCompleted := "keep"
	if req.OnRunCompleted != "" {
		onRunCompleted = req.OnRunCompleted
	}

	payload := map[string]interface{}{
		"input":            req.Input,
		"config":           req.Config,
		"metadata":         req.Metadata,
		"context":          req.Context,
		"webhook":          req.Webhook,
		"interrupt_before": req.InterruptBefore,
		"interrupt_after":  req.InterruptAfter,
	}

	now := time.Now().UTC()
	nextRun, err := service.ComputeNextRun(req.Schedule, tz, now)
	if err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	cron := &postgres.CronJob{
		AssistantID:    req.AssistantID,
		ThreadID:       threadID,
		Schedule:       req.Schedule,
		Timezone:       tz,
		Payload:        payload,
		Metadata:       req.Metadata,
		Enabled:        enabled,
		OnRunCompleted: onRunCompleted,
		EndTime:        req.EndTime,
		NextRunDate:    &nextRun,
	}

	cronID, err := h.repo.Create(c.Request().Context(), cron)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "internal_error", Message: err.Error(),
		})
	}

	created, err := h.repo.GetByID(c.Request().Context(), cronID)
	if err != nil || created == nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "internal_error", Message: "failed to retrieve created cron",
		})
	}

	return c.JSON(http.StatusCreated, cronToResponse(created))
}

// CreateStatelessCron creates a cron job without a thread.
// POST /runs/crons
func (h *CronHandler) CreateStatelessCron(c echo.Context) error {
	return h.createCron(c, nil)
}

// CreateThreadCron creates a cron job for a specific thread.
// POST /threads/:thread_id/runs/crons
func (h *CronHandler) CreateThreadCron(c echo.Context) error {
	threadID := c.Param("thread_id")
	if threadID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: "thread_id is required",
		})
	}
	return h.createCron(c, &threadID)
}

// DeleteCron deletes a cron job.
// DELETE /runs/crons/:cron_id
func (h *CronHandler) DeleteCron(c echo.Context) error {
	cronID := c.Param("cron_id")
	if cronID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: "cron_id is required",
		})
	}

	if err := h.repo.Delete(c.Request().Context(), cronID); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "internal_error", Message: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// UpdateCron updates a cron job.
// PATCH /runs/crons/:cron_id
func (h *CronHandler) UpdateCron(c echo.Context) error {
	cronID := c.Param("cron_id")
	if cronID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: "cron_id is required",
		})
	}

	var req dto.UpdateCronRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	updates := map[string]interface{}{}

	if req.Schedule != nil {
		if err := service.ValidateSchedule(*req.Schedule); err != nil {
			return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error: "invalid_request", Message: err.Error(),
			})
		}
		updates["schedule"] = *req.Schedule
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}
	if req.OnRunCompleted != nil {
		updates["on_run_completed"] = *req.OnRunCompleted
	}
	if req.EndTime != nil {
		updates["end_time"] = *req.EndTime
	}
	if req.Webhook != nil {
		updates["payload"] = map[string]interface{}{"webhook": *req.Webhook}
	}
	if req.Metadata != nil {
		updates["metadata"] = req.Metadata
	}

	if req.Schedule != nil || req.Timezone != nil {
		existing, err := h.repo.GetByID(c.Request().Context(), cronID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error: "internal_error", Message: err.Error(),
			})
		}
		if existing == nil {
			return c.JSON(http.StatusNotFound, dto.ErrorResponse{
				Error: "not_found", Message: "cron not found",
			})
		}

		sched := existing.Schedule
		if req.Schedule != nil {
			sched = *req.Schedule
		}
		tz := existing.Timezone
		if req.Timezone != nil {
			tz = *req.Timezone
		}

		nextRun, err := service.ComputeNextRun(sched, tz, time.Now().UTC())
		if err != nil {
			return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error: "invalid_request", Message: err.Error(),
			})
		}
		updates["next_run_date"] = nextRun
	}

	updated, err := h.repo.Update(c.Request().Context(), cronID, updates)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "internal_error", Message: err.Error(),
		})
	}
	if updated == nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error: "not_found", Message: "cron not found",
		})
	}

	return c.JSON(http.StatusOK, cronToResponse(updated))
}

// SearchCrons searches for cron jobs.
// POST /runs/crons/search
func (h *CronHandler) SearchCrons(c echo.Context) error {
	var req dto.SearchCronsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	crons, err := h.repo.Search(c.Request().Context(), req.AssistantID, req.ThreadID, req.Enabled, req.Limit, req.Offset, req.SortBy, req.SortOrder)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "internal_error", Message: err.Error(),
		})
	}

	resp := make([]dto.CronResponse, 0, len(crons))
	for _, cr := range crons {
		resp = append(resp, cronToResponse(&cr))
	}

	return c.JSON(http.StatusOK, resp)
}

// CountCrons counts cron jobs.
// POST /runs/crons/count
func (h *CronHandler) CountCrons(c echo.Context) error {
	var req dto.CountCronsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	count, err := h.repo.Count(c.Request().Context(), req.AssistantID, req.ThreadID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "internal_error", Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, dto.CountResponse{Count: count})
}
