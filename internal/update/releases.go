package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fulgidus/revoco/internal/version"
)

// Release represents a GitHub release with metadata and assets.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []Asset   `json:"assets"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
}

// Asset represents a downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// FetchLatestStableRelease fetches the latest stable (non-prerelease) release
// from GitHub's /releases/latest endpoint.
func FetchLatestStableRelease(ctx context.Context, apiBase, owner, repo string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBase, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "revoco-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	return &release, nil
}

// FetchLatestDevRelease fetches all releases from GitHub and returns the highest
// version development release (containing "-dev-" in the tag). It handles pagination
// to ensure all releases are checked.
func FetchLatestDevRelease(ctx context.Context, apiBase, owner, repo string) (*Release, error) {
	var allReleases []Release
	nextURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", apiBase, owner, repo)

	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", nextURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("User-Agent", "revoco-updater")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var pageReleases []Release
		if err := json.NewDecoder(resp.Body).Decode(&pageReleases); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to parse releases: %w", err)
		}
		resp.Body.Close()

		allReleases = append(allReleases, pageReleases...)

		// Check for pagination
		nextURL = parseNextLink(resp.Header.Get("Link"))
	}

	// Filter dev releases
	var devReleases []Release
	for _, rel := range allReleases {
		if version.IsDevVersion(rel.TagName) {
			devReleases = append(devReleases, rel)
		}
	}

	if len(devReleases) == 0 {
		return nil, fmt.Errorf("no development releases found")
	}

	// Find highest version using semver comparison
	highest := &devReleases[0]
	for i := 1; i < len(devReleases); i++ {
		candidate := &devReleases[i]
		isNewer, err := version.IsNewer(candidate.TagName, highest.TagName)
		if err != nil {
			// Skip invalid versions
			continue
		}
		if isNewer {
			highest = candidate
		}
	}

	return highest, nil
}

// FetchLatestRelease dispatches to the appropriate fetch function based on the
// specified channel ("stable" or "dev").
func FetchLatestRelease(ctx context.Context, apiBase, owner, repo, channel string) (*Release, error) {
	switch channel {
	case "stable":
		return FetchLatestStableRelease(ctx, apiBase, owner, repo)
	case "dev":
		return FetchLatestDevRelease(ctx, apiBase, owner, repo)
	default:
		return nil, fmt.Errorf("unknown channel: %s (must be 'stable' or 'dev')", channel)
	}
}

// parseNextLink extracts the "next" URL from a GitHub API Link header.
// Returns empty string if no next link exists.
func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	// Link header format: <url>; rel="next", <url>; rel="last"
	links := strings.Split(linkHeader, ",")
	for _, link := range links {
		parts := strings.Split(strings.TrimSpace(link), ";")
		if len(parts) != 2 {
			continue
		}

		url := strings.Trim(strings.TrimSpace(parts[0]), "<>")
		rel := strings.TrimSpace(parts[1])

		if strings.Contains(rel, `rel="next"`) {
			return url
		}
	}

	return ""
}
