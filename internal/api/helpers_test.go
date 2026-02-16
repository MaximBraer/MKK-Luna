package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"MKK-Luna/internal/service"
)

func TestMapServiceError_AllKnown(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "not found", err: service.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "forbidden", err: service.ErrForbidden, wantStatus: http.StatusForbidden},
		{name: "conflict", err: service.ErrConflict, wantStatus: http.StatusConflict},
		{name: "bad request", err: service.ErrBadRequest, wantStatus: http.StatusBadRequest},
		{name: "unknown", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mapped := mapServiceError(w, tt.err)
			if !mapped {
				t.Fatalf("expected mapped=true")
			}
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d want=%d", w.Code, tt.wantStatus)
			}
			if !strings.Contains(w.Body.String(), `"status":"error"`) {
				t.Fatalf("unexpected body: %s", w.Body.String())
			}
		})
	}
}

func TestMapServiceError_Nil(t *testing.T) {
	w := httptest.NewRecorder()
	if mapped := mapServiceError(w, nil); mapped {
		t.Fatalf("expected mapped=false for nil")
	}
	if w.Code != 200 {
		t.Fatalf("unexpected status for nil mapping: %d", w.Code)
	}
}
