package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestStableRelease(t *testing.T) {
	tests := []struct {
		name           string
		mockStatusCode int
		mockResponse   string
		expectError    bool
		expectedTag    string
	}{
		{
			name:           "successful fetch",
			mockStatusCode: http.StatusOK,
			mockResponse: `{
				"tag_name": "v1.2.3",
				"name": "Release 1.2.3",
				"draft": false,
				"prerelease": false,
				"body": "Release notes",
				"published_at": "2026-03-01T10:00:00Z"
			}`,
			expectError: false,
			expectedTag: "v1.2.3",
		},
		{
			name:           "404 not found",
			mockStatusCode: http.StatusNotFound,
			mockResponse:   `{"message": "Not Found"}`,
			expectError:    true,
		},
		{
			name:           "500 server error",
			mockStatusCode: http.StatusInternalServerError,
			mockResponse:   `{"message": "Internal Server Error"}`,
			expectError:    true,
		},
		{
			name:           "invalid json response",
			mockStatusCode: http.StatusOK,
			mockResponse:   `{invalid json}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request path
				expectedPath := "/repos/owner/repo/releases/latest"
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
				}

				// Verify headers
				if accept := r.Header.Get("Accept"); accept != "application/vnd.github.v3+json" {
					t.Errorf("unexpected Accept header: got %s", accept)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				w.Write([]byte(tt.mockResponse))
			}))
			defer server.Close()

			ctx := context.Background()
			release, err := FetchLatestStableRelease(ctx, server.URL, "owner", "repo")

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if release == nil {
					t.Fatal("expected release but got nil")
				}
				if release.TagName != tt.expectedTag {
					t.Errorf("unexpected tag: got %s, want %s", release.TagName, tt.expectedTag)
				}
			}
		})
	}
}

func TestFetchLatestDevRelease(t *testing.T) {
	tests := []struct {
		name        string
		mockPages   []string
		mockLinks   []string
		expectError bool
		expectedTag string
	}{
		{
			name: "single page with dev releases",
			mockPages: []string{
				`[
					{"tag_name": "v1.0.0", "name": "Release 1.0.0", "prerelease": false, "published_at": "2026-03-01T10:00:00Z"},
					{"tag_name": "v1.1.0-dev-2026-03-05T12-00-00", "name": "Dev 1.1.0", "prerelease": true, "published_at": "2026-03-05T12:00:00Z"},
					{"tag_name": "v1.0.0-dev-2026-03-04T10-00-00", "name": "Dev 1.0.0", "prerelease": true, "published_at": "2026-03-04T10:00:00Z"}
				]`,
			},
			mockLinks:   []string{""},
			expectError: false,
			expectedTag: "v1.1.0-dev-2026-03-05T12-00-00",
		},
		{
			name: "multiple pages with dev releases",
			mockPages: []string{
				`[
					{"tag_name": "v1.0.0", "name": "Release 1.0.0", "prerelease": false, "published_at": "2026-03-01T10:00:00Z"},
					{"tag_name": "v1.0.0-dev-2026-03-04T10-00-00", "name": "Dev 1.0.0", "prerelease": true, "published_at": "2026-03-04T10:00:00Z"}
				]`,
				`[
					{"tag_name": "v1.2.0-dev-2026-03-05T14-00-00", "name": "Dev 1.2.0", "prerelease": true, "published_at": "2026-03-05T14:00:00Z"},
					{"tag_name": "v0.9.0-dev-2026-03-03T09-00-00", "name": "Dev 0.9.0", "prerelease": true, "published_at": "2026-03-03T09:00:00Z"}
				]`,
			},
			mockLinks: []string{
				`<http://example.com/releases?page=2>; rel="next"`,
				"",
			},
			expectError: false,
			expectedTag: "v1.2.0-dev-2026-03-05T14-00-00",
		},
		{
			name: "no dev releases found",
			mockPages: []string{
				`[
					{"tag_name": "v1.0.0", "name": "Release 1.0.0", "prerelease": false, "published_at": "2026-03-01T10:00:00Z"},
					{"tag_name": "v1.1.0", "name": "Release 1.1.0", "prerelease": false, "published_at": "2026-03-02T10:00:00Z"}
				]`,
			},
			mockLinks:   []string{""},
			expectError: true,
		},
		{
			name: "empty releases list",
			mockPages: []string{
				`[]`,
			},
			mockLinks:   []string{""},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageIndex := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request path starts with expected path
				// Verify request path - pagination URLs may not have the full path
				validPaths := []string{"/repos/owner/repo/releases", "/repos/owner/repo/releases?page=2"}
				isValidPath := false
				for _, validPath := range validPaths {
					if httpPathHasPrefix(r.URL.Path, validPath) {
						isValidPath = true
						break
					}
				}
				if !isValidPath {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}

				// Verify headers
				if accept := r.Header.Get("Accept"); accept != "application/vnd.github.v3+json" {
					t.Errorf("unexpected Accept header: got %s", accept)
				}

				// Return the appropriate page
				if pageIndex < len(tt.mockPages) {
					w.Header().Set("Content-Type", "application/json")
					if tt.mockLinks[pageIndex] != "" {
						// Replace example.com with actual server URL
						linkHeader := httpReplaceHost(tt.mockLinks[pageIndex], r.Host)
						w.Header().Set("Link", linkHeader)
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(tt.mockPages[pageIndex]))
					pageIndex++
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			ctx := context.Background()
			release, err := FetchLatestDevRelease(ctx, server.URL, "owner", "repo")

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if release == nil {
					t.Fatal("expected release but got nil")
				}
				if release.TagName != tt.expectedTag {
					t.Errorf("unexpected tag: got %s, want %s", release.TagName, tt.expectedTag)
				}
			}
		})
	}
}

func TestFetchLatestRelease_Dispatcher(t *testing.T) {
	tests := []struct {
		name        string
		channel     string
		mockData    string
		expectedTag string
	}{
		{
			name:    "stable channel",
			channel: "stable",
			mockData: `{
				"tag_name": "v1.0.0",
				"name": "Stable Release",
				"prerelease": false,
				"published_at": "2026-03-01T10:00:00Z"
			}`,
			expectedTag: "v1.0.0",
		},
		{
			name:    "dev channel",
			channel: "dev",
			mockData: `[
				{"tag_name": "v1.0.0", "name": "Release", "prerelease": false, "published_at": "2026-03-01T10:00:00Z"},
				{"tag_name": "v1.1.0-dev-2026-03-05T12-00-00", "name": "Dev", "prerelease": true, "published_at": "2026-03-05T12:00:00Z"}
			]`,
			expectedTag: "v1.1.0-dev-2026-03-05T12-00-00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.mockData))
			}))
			defer server.Close()

			ctx := context.Background()
			release, err := FetchLatestRelease(ctx, server.URL, "owner", "repo", tt.channel)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if release == nil {
				t.Fatal("expected release but got nil")
			}
			if release.TagName != tt.expectedTag {
				t.Errorf("unexpected tag: got %s, want %s", release.TagName, tt.expectedTag)
			}
		})
	}
}

func TestFetchLatestRelease_UnknownChannel(t *testing.T) {
	ctx := context.Background()
	_, err := FetchLatestRelease(ctx, "http://example.com", "owner", "repo", "unknown")

	if err == nil {
		t.Error("expected error for unknown channel but got none")
	}
}

// Helper functions for test setup
func httpPathHasPrefix(path, prefix string) bool {
	return len(path) >= len(prefix) && path[:len(prefix)] == prefix
}

func httpReplaceHost(linkHeader, newHost string) string {
	// Generate proper pagination URL for test server
	return fmt.Sprintf("<http://%s/repos/owner/repo/releases?page=2>; rel=\"next\"", newHost)
}
