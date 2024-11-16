package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openperouter/openperouter/internal/reload"
)

func TestHandler(t *testing.T) {
	reloadSucceeds := func(_ string) error {
		return nil
	}

	reloadFails := func(_ string) error {
		return errors.New("failed")
	}

	tests := []struct {
		name       string
		reloadMock func(string) error
		method     string
		httpStatus int
	}{
		{
			"succeeds",
			reloadSucceeds,
			http.MethodPost,
			200,
		},
		{
			"wrong method",
			reloadSucceeds,
			http.MethodGet,
			http.StatusBadRequest,
		},
		{
			"reload fails",
			reloadFails,
			http.MethodPost,
			http.StatusInternalServerError,
		},
	}

	t.Cleanup(func() {
		reloadConfig = reload.Config
	})
	for _, tc := range tests {
		reloadConfig = tc.reloadMock
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tc.method, "/", nil)
			handler := http.HandlerFunc(reloadHandler)

			handler.ServeHTTP(w, req)
			res := w.Result()
			res.Body.Close()
			if res.StatusCode != tc.httpStatus {
				t.Fatalf("expecting %d, got %d", res.StatusCode, tc.httpStatus)
			}
		})
	}
}