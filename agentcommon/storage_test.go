package agentcommon

import (
	"errors"
	"net/http"
	"testing"
)

func TestJfrogClientResponseStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   int
		wantParsed bool
	}{
		{
			name:       "404 from FolderInfo",
			err:        errors.New("server response: 404 Not Found"),
			wantCode:   http.StatusNotFound,
			wantParsed: true,
		},
		{
			name:       "404 with JSON body",
			err:        errors.New("server response: 404 Not Found\n{\"errors\":[]}"),
			wantCode:   http.StatusNotFound,
			wantParsed: true,
		},
		{
			name:       "403 forbidden",
			err:        errors.New("server response: 403 Forbidden"),
			wantCode:   http.StatusForbidden,
			wantParsed: true,
		},
		{
			name:       "repo name containing 404 is not a response error",
			err:        errors.New("failed to access repo 404-something"),
			wantParsed: false,
		},
		{
			name:       "unrelated error",
			err:        errors.New("connection refused"),
			wantParsed: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, ok := jfrogClientResponseStatusCode(tt.err)
			if ok != tt.wantParsed {
				t.Fatalf("parsed = %v, want %v", ok, tt.wantParsed)
			}
			if ok && code != tt.wantCode {
				t.Fatalf("code = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

func TestIsNotFoundErr(t *testing.T) {
	if !isNotFoundErr(errors.New("server response: 404 Not Found")) {
		t.Fatal("expected 404 response error to be not-found")
	}
	if isNotFoundErr(errors.New("server response: 403 Forbidden")) {
		t.Fatal("expected 403 response error not to be not-found")
	}
	if isNotFoundErr(errors.New("repo 404-test failed")) {
		t.Fatal("expected unrelated error not to be not-found")
	}
}
