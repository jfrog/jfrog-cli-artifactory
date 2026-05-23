package common

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

type statusCodeError struct {
	code int
}

func (e statusCodeError) Error() string {
	return fmt.Sprintf("http status %d", e.code)
}

func (e statusCodeError) StatusCode() int {
	return e.code
}

func TestJfrogClientHTTPStatusCode(t *testing.T) {
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
			name:       "wrapped GenerateResponseError",
			err:        fmt.Errorf("folder info: %w", errors.New("server response: 404 Not Found")),
			wantCode:   http.StatusNotFound,
			wantParsed: true,
		},
		{
			name:       "outer message does not parse without unwrap",
			err:        fmt.Errorf("folder info failed: %w", errors.New("server response: 404 Not Found")),
			wantCode:   http.StatusNotFound,
			wantParsed: true,
		},
		{
			name:       "invalid status code in response prefix",
			err:        errors.New("server response: 99 Invalid"),
			wantParsed: false,
		},
		{
			name:       "StatusCode method",
			err:        statusCodeError{code: http.StatusNotFound},
			wantCode:   http.StatusNotFound,
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
			code, ok := jfrogClientHTTPStatusCode(tt.err)
			if ok != tt.wantParsed {
				t.Fatalf("parsed = %v, want %v", ok, tt.wantParsed)
			}
			if ok && code != tt.wantCode {
				t.Fatalf("code = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

func TestPackageVersionExistsUnknownError(t *testing.T) {
	err := fmt.Errorf("%w: %w", ErrVersionExistenceUnknown, errors.New("connection refused"))
	if !errors.Is(err, ErrVersionExistenceUnknown) {
		t.Fatal("expected ErrVersionExistenceUnknown")
	}
	if _, ok := jfrogClientHTTPStatusCode(errors.New("connection refused")); ok {
		t.Fatal("expected unparseable error")
	}
}
