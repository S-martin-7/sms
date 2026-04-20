package httpx_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/S-martin-7/sms/internal/httpx"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	httpx.WriteJSON(w, 201, map[string]string{"a": "b"})
	if w.Code != 201 {
		t.Errorf("code = %d, want 201", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	var got map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["a"] != "b" {
		t.Errorf("body = %v", got)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	httpx.WriteError(w, 401, "unauthorized", "no api key")
	if w.Code != 401 {
		t.Errorf("code = %d, want 401", w.Code)
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Error.Code != "unauthorized" || got.Error.Message != "no api key" {
		t.Errorf("body = %+v", got)
	}
}
