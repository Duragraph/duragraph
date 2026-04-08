package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/labstack/echo/v4"
)

func TestCronHandler_CreateStatelessCron_Validation(t *testing.T) {
	e := echo.New()
	h := NewCronHandler(nil)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing assistant_id",
			body:       `{"schedule":"0 0 * * *"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "assistant_id is required",
		},
		{
			name:       "missing schedule",
			body:       `{"assistant_id":"a1"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "schedule is required",
		},
		{
			name:       "invalid schedule",
			body:       `{"assistant_id":"a1","schedule":"not-a-cron"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid cron schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/runs/crons", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.CreateStatelessCron(c)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var resp dto.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if !strings.Contains(resp.Message, tt.wantError) {
				t.Errorf("message = %q, want to contain %q", resp.Message, tt.wantError)
			}
		})
	}
}

func TestCronHandler_CreateThreadCron_MissingThreadID(t *testing.T) {
	e := echo.New()
	h := NewCronHandler(nil)

	req := httptest.NewRequest(http.MethodPost, "/threads//runs/crons", strings.NewReader(`{"assistant_id":"a1","schedule":"0 0 * * *"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("thread_id")
	c.SetParamValues("")

	err := h.CreateThreadCron(c)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCronHandler_DeleteCron_MissingID(t *testing.T) {
	e := echo.New()
	h := NewCronHandler(nil)

	req := httptest.NewRequest(http.MethodDelete, "/runs/crons/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("cron_id")
	c.SetParamValues("")

	err := h.DeleteCron(c)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCronHandler_UpdateCron_MissingID(t *testing.T) {
	e := echo.New()
	h := NewCronHandler(nil)

	req := httptest.NewRequest(http.MethodPatch, "/runs/crons/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("cron_id")
	c.SetParamValues("")

	err := h.UpdateCron(c)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCronHandler_UpdateCron_InvalidSchedule(t *testing.T) {
	e := echo.New()
	h := NewCronHandler(nil)

	badSched := "not-valid"
	req := httptest.NewRequest(http.MethodPatch, "/runs/crons/abc", strings.NewReader(`{"schedule":"not-valid"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("cron_id")
	c.SetParamValues("abc")

	_ = badSched

	err := h.UpdateCron(c)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
