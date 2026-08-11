package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	tagRegex     = regexp.MustCompile(`<[^>]*>`)
	snippetRegex = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>|<div[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</div>`)
	titleRegex   = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__url[^"]*"[^>]*>(.*?)</a>|<h2[^>]*class="[^"]*result__title[^"]*"[^>]*>(.*?)</h2>`)
)

type ddgInstantAnswer struct {
	AbstractText string `json:"AbstractText"`
	Heading      string `json:"Heading"`
	Answer       string `json:"Answer"`
	RelatedTopics []struct {
		Text string `json:"Text"`
	} `json:"RelatedTopics"`
}

// SearchLiveWeb executes a real-time web search for any query to ground the LLM with live internet data.
func SearchLiveWeb(ctx context.Context, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	searchCtx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
	defer cancel()

	var results []string

	// 1. Fetch live web results via DuckDuckGo HTML Search
	htmlResults := fetchDDGHTMLSearch(searchCtx, query)
	if len(htmlResults) > 0 {
		results = append(results, htmlResults...)
	}

	// 2. Fetch Instant Answer API if available
	if len(results) < 2 {
		if instant := fetchDDGInstantAnswer(searchCtx, query); instant != "" {
			results = append(results, instant)
		}
	}

	if len(results) == 0 {
		return ""
	}

	// Limit to top 4 clean snippets
	if len(results) > 4 {
		results = results[:4]
	}

	var sb strings.Builder
	sb.WriteString("\n\n[Live Real-Time Web Search Results (Current & Grounded)]:\n")
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r))
	}

	return sb.String()
}

func fetchDDGHTMLSearch(ctx context.Context, query string) []string {
	endpoint := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}

	// Modern browser User-Agent to ensure reliable response
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	body := string(bodyBytes)
	matches := snippetRegex.FindAllStringSubmatch(body, 6)

	var snippets []string
	for _, m := range matches {
		raw := m[1]
		if raw == "" && len(m) > 2 {
			raw = m[2]
		}
		clean := cleanHTMLText(raw)
		if len(clean) > 20 && !containsDuplicate(snippets, clean) {
			snippets = append(snippets, clean)
		}
	}

	return snippets
}

func fetchDDGInstantAnswer(ctx context.Context, query string) string {
	endpoint := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "ShellChat/1.0")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var data ddgInstantAnswer
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}

	if data.AbstractText != "" {
		return fmt.Sprintf("%s: %s", data.Heading, data.AbstractText)
	}
	if data.Answer != "" {
		return data.Answer
	}
	if len(data.RelatedTopics) > 0 && data.RelatedTopics[0].Text != "" {
		return data.RelatedTopics[0].Text
	}

	return ""
}

func cleanHTMLText(raw string) string {
	// Strip HTML tags
	noTags := tagRegex.ReplaceAllString(raw, " ")
	// Unescape HTML entities (&amp;, &quot;, &#x27;, etc.)
	unescaped := html.UnescapeString(noTags)
	// Normalize whitespace
	fields := strings.Fields(unescaped)
	return strings.Join(fields, " ")
}

func containsDuplicate(list []string, item string) bool {
	for _, s := range list {
		if strings.EqualFold(s, item) || strings.Contains(s, item) || strings.Contains(item, s) {
			return true
		}
	}
	return false
}
