package cdp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Page represents a CDP debuggable page.
type Page struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	Type                 string `json:"type"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// domainHints maps lowercase app name keywords to URL domains for matching.
var domainHints = map[string][]string{
	"instagram": {"instagram.com"},
	"linkedin":  {"linkedin.com"},
	"whatsapp":  {"web.whatsapp.com", "whatsapp.com"},
	"twitter":   {"twitter.com", "x.com"},
	"facebook":  {"facebook.com"},
	"slack":     {"slack.com"},
	"discord":   {"discord.com"},
	"teams":     {"teams.microsoft.com"},
	"spotify":   {"open.spotify.com"},
	"youtube":   {"youtube.com"},
	"github":    {"github.com"},
	"gmail":     {"mail.google.com"},
	"google":    {"google.com"},
	"reddit":    {"reddit.com"},
	"telegram":  {"web.telegram.org", "telegram.org"},
	"notion":    {"notion.so"},
	"figma":     {"figma.com"},
}

// DetectCDPPort checks if a process has a WebView2/Chromium child process
// with --remote-debugging-port. Returns the port if found.
func DetectCDPPort(pid uint32) (int, error) {
	psScript := fmt.Sprintf(`
		$procs = Get-CimInstance Win32_Process | Where-Object {
			($_.Name -eq 'msedgewebview2.exe' -or $_.Name -eq 'chrome.exe' -or $_.Name -eq 'msedge.exe') -and
			$_.CommandLine -match '--remote-debugging-port=(\d+)'
		}
		if ($procs) {
			foreach ($p in $procs) {
				if ($p.CommandLine -match '--remote-debugging-port=(\d+)') {
					$Matches[1]
					break
				}
			}
		}
	`)

	out, err := exec.Command("powershell", "-NoProfile", "-Command", psScript).Output()
	if err != nil {
		return 0, fmt.Errorf("detect cdp port: %w", err)
	}

	portStr := strings.TrimSpace(string(out))
	if portStr == "" {
		return 0, fmt.Errorf("no CDP port found for PID %d", pid)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	log.Printf("[CDP] Detected port %d for PID %d", port, pid)
	return port, nil
}

// deriveDomains extracts URL domain hints from a titleHint string.
func deriveDomains(titleHint string) []string {
	lower := strings.ToLower(titleHint)
	for key, domains := range domainHints {
		if strings.Contains(lower, key) {
			return domains
		}
	}
	return nil
}

// ListPages queries the CDP /json endpoint and returns all pages.
func ListPages(port int) ([]Page, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	url := fmt.Sprintf("http://localhost:%d/json", port)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("cdp endpoint query %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cdp read response: %w", err)
	}

	var pages []Page
	if err := json.Unmarshal(body, &pages); err != nil {
		return nil, fmt.Errorf("cdp parse pages: %w", err)
	}
	return pages, nil
}

// FindCDPEndpoint queries the CDP /json endpoint and returns the WebSocket URL
// for the page matching by URL domain (derived from titleHint) or title.
func FindCDPEndpoint(port int, titleHint string) (string, error) {
	pages, err := ListPages(port)
	if err != nil {
		return "", err
	}

	if len(pages) == 0 {
		return "", fmt.Errorf("no CDP pages found on port %d", port)
	}

	log.Printf("[CDP] Found %d pages on port %d", len(pages), port)

	if titleHint == "" {
		for i, p := range pages {
			if p.Type == "page" {
				return returnPage(&pages[i])
			}
		}
		return "", fmt.Errorf("no CDP pages of type 'page' on port %d", port)
	}

	// Try domain matching first
	domains := deriveDomains(titleHint)
	if len(domains) > 0 {
		for i, p := range pages {
			if p.Type != "page" {
				continue
			}
			pageLower := strings.ToLower(p.URL)
			for _, domain := range domains {
				if strings.Contains(pageLower, domain) {
					log.Printf("[CDP] Matched by domain %q: %q (%s)", domain, p.Title, p.URL)
					return returnPage(&pages[i])
				}
			}
		}
	}

	// Fallback: match by title
	titleLower := strings.ToLower(titleHint)
	for i, p := range pages {
		if p.Type != "page" {
			continue
		}
		if strings.Contains(strings.ToLower(p.Title), titleLower) ||
			strings.Contains(strings.ToLower(p.URL), titleLower) {
			log.Printf("[CDP] Matched by title %q: %q (%s)", titleHint, p.Title, p.URL)
			return returnPage(&pages[i])
		}
	}

	return "", fmt.Errorf("no CDP page matching %q on port %d (tried domains=%v)", titleHint, port, domains)
}

func returnPage(p *Page) (string, error) {
	if p.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("page %q has no webSocketDebuggerUrl", p.Title)
	}
	log.Printf("[CDP] Selected page: %q (%s)", p.Title, p.URL)
	return p.WebSocketDebuggerURL, nil
}

// ScanCDPPorts scans ports 9222-9400 for any responding CDP /json endpoints.
// Returns a map of port -> pages found.
func ScanCDPPorts() map[int][]Page {
	result := make(map[int][]Page)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for port := 9222; port <= 9400; port++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			pages, err := ListPages(p)
			if err != nil {
				return
			}
			if len(pages) > 0 {
				mu.Lock()
				result[p] = pages
				mu.Unlock()
			}
		}(port)
	}
	wg.Wait()

	if len(result) > 0 {
		ports := make([]int, 0, len(result))
		for p := range result {
			ports = append(ports, p)
		}
		log.Printf("[CDP] Port scan found %d active ports: %v", len(result), ports)
	}

	return result
}

// IsPortAlive checks if a CDP port is responding.
func IsPortAlive(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/json/version", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// containsAny checks if s contains any of the items.
func containsAny(s string, items []string) bool {
	for _, item := range items {
		if strings.Contains(s, item) {
			return true
		}
	}
	return false
}
