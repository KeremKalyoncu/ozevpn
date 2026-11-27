package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AppVersion is the current OzeVPN semantic version (overrideable via -ldflags).
var AppVersion = "v6.1.2"

const (
	latestReleaseAPI = "https://api.github.com/repos/KeremKalyoncu/ozevpn/releases/latest"
	installOneLiner  = "irm https://raw.githubusercontent.com/KeremKalyoncu/ozevpn/main/scripts/install.ps1 | iex"
)

type updateCheckResult struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
	PublishedAt     string `json:"published_at,omitempty"`
	InstallCmd      string `json:"install_cmd"`
}

// compareSemVer compares two vX.Y.Z strings; returns -1 if a<b, 0 if equal, +1 if a>b.
func compareSemVer(a, b string) int {
	pa := parseSemVerParts(a)
	pb := parseSemVerParts(b)
	for i := range 3 {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseSemVerParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		if n, err := strconv.Atoi(parts[i]); err == nil && n >= 0 {
			out[i] = n
		}
	}
	return out
}

// handleUpdateCheck performs an explicit, user-triggered check against GitHub Releases.
// It is never invoked automatically on startup to preserve OzeVPN's zero-telemetry guarantee.
func (s *apiServer) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "OzeVPN-UpdateCheck/"+AppVersion)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "GitHub sürüm sunucusuna ulaşılamadı: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil || resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "GitHub sürüm bilgisi alınamadı (HTTP " + strconv.Itoa(resp.StatusCode) + ")"})
		return
	}
	var gh struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.Unmarshal(body, &gh); err != nil || strings.TrimSpace(gh.TagName) == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Geçersiz GitHub sürüm yanıtı"})
		return
	}
	relURL := strings.TrimSpace(gh.HTMLURL)
	if !strings.HasPrefix(relURL, "https://github.com/KeremKalyoncu/ozevpn/") {
		relURL = "https://github.com/KeremKalyoncu/ozevpn/releases"
	}
	writeJSON(w, http.StatusOK, updateCheckResult{
		Current:         AppVersion,
		Latest:          gh.TagName,
		UpdateAvailable: compareSemVer(AppVersion, gh.TagName) < 0,
		ReleaseURL:      relURL,
		PublishedAt:     gh.PublishedAt,
		InstallCmd:      installOneLiner,
	})
}
