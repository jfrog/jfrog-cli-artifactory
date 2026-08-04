package common

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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

func TestIsHTTPNotFound(t *testing.T) {
	if !IsHTTPNotFound(errors.New("server response: 404 Not Found")) {
		t.Fatal("expected 404")
	}
	if IsHTTPNotFound(errors.New("server response: 403 Forbidden")) {
		t.Fatal("expected not 404")
	}
	if IsHTTPNotFound(errors.New("connection refused")) {
		t.Fatal("expected unparseable error")
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

func TestLatestOrFallback_FallsBackOnUnparsableVersions(t *testing.T) {
	versions := []string{"not-semver", "also-not-semver"}
	got := latestOrFallback(versions)
	want := versions[len(versions)-1]
	if got != want {
		t.Fatalf("got %q, want last element %q", got, want)
	}
}

func TestPromptForNewVersion_EmptyInputAborts(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	os.Stdin = r

	go func() {
		defer func() { _ = w.Close() }()
		_, _ = w.WriteString("\n")
	}()

	_, err = promptForNewVersion()
	if err == nil {
		t.Fatal("expected an error for empty version input")
	}
	if !strings.Contains(err.Error(), "no version provided, aborting") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchExistingVersionStrings_ToleratesListError(t *testing.T) {
	got := fetchExistingVersionStrings(ResolveMissingVersionOpts{
		ListVersions: func(*config.ServerDetails, string, string) ([]PublishableVersion, error) {
			return nil, errors.New("network error")
		},
	})
	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice on lookup error", got)
	}
}
