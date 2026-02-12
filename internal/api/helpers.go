package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/api/response"
)

func writeJSONBytes(w http.ResponseWriter, status int, payload []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func mapServiceError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case err == service.ErrNotFound:
		response.Error(w, http.StatusNotFound, "not found")
		return true
	case err == service.ErrForbidden:
		response.Error(w, http.StatusForbidden, "forbidden")
		return true
	case err == service.ErrConflict:
		response.Error(w, http.StatusConflict, "conflict")
		return true
	case err == service.ErrBadRequest:
		response.Error(w, http.StatusBadRequest, "invalid request")
		return true
	default:
		response.Error(w, http.StatusInternalServerError, "internal error")
		return true
	}
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
