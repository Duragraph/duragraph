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

func TestStoreHandler_PutItem_Validation(t *testing.T) {
	e := echo.New()
	h := NewStoreHandler(nil)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing namespace",
			body:       `{"key":"k","value":{}}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "namespace is required",
		},
		{
			name:       "empty namespace",
			body:       `{"namespace":[],"key":"k","value":{}}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "namespace is required",
		},
		{
			name:       "missing key",
			body:       `{"namespace":["a"],"value":{}}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/store/items", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.PutItem(c)
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
			if resp.Message != tt.wantError {
				t.Errorf("message = %q, want %q", resp.Message, tt.wantError)
			}
		})
	}
}

func TestStoreHandler_GetItem_Validation(t *testing.T) {
	e := echo.New()
	h := NewStoreHandler(nil)

	tests := []struct {
		name       string
		namespace  string
		key        string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing namespace",
			namespace:  "",
			key:        "k",
			wantStatus: http.StatusBadRequest,
			wantError:  "namespace query parameter is required",
		},
		{
			name:       "missing key",
			namespace:  "a.b",
			key:        "",
			wantStatus: http.StatusBadRequest,
			wantError:  "key query parameter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "/store/items?"
			if tt.namespace != "" {
				target += "namespace=" + tt.namespace + "&"
			}
			if tt.key != "" {
				target += "key=" + tt.key
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.GetItem(c)
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
			if resp.Message != tt.wantError {
				t.Errorf("message = %q, want %q", resp.Message, tt.wantError)
			}
		})
	}
}

func TestStoreHandler_DeleteItem_Validation(t *testing.T) {
	e := echo.New()
	h := NewStoreHandler(nil)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing namespace",
			body:       `{"key":"k"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "namespace is required",
		},
		{
			name:       "missing key",
			body:       `{"namespace":["a"]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/store/items", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.DeleteItem(c)
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
			if resp.Message != tt.wantError {
				t.Errorf("message = %q, want %q", resp.Message, tt.wantError)
			}
		})
	}
}
