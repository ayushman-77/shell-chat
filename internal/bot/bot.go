package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultGeminiModel is the default fast and search-grounded model for Google Gemini.
	DefaultGeminiModel = "gemini-1.5-flash"
	// CooldownDuration is the minimum time between /ask requests per user.
	CooldownDuration = 3 * time.Second
	// MaxChannelMemoryTurns is the maximum number of recent conversation messages remembered per channel.
	MaxChannelMemoryTurns = 16
	// MaxUserFacts is the maximum number of individual facts/preferences stored per user.
	MaxUserFacts = 8
)

// AIConfig holds the credentials for the AI engine.
type AIConfig struct {
	GeminiAPIKey string
	GeminiModel  string
}

// CooldownTracker keeps track of per-user cooldowns to prevent spam.
type CooldownTracker struct {
	mu       sync.Mutex
	lastAsks map[int64]time.Time
}

var globalCooldown = &CooldownTracker{
	lastAsks: make(map[int64]time.Time),
}

// UserProfile stores individual user details, preferences, and facts learned across interactions.
type UserProfile struct {
	UserID   int64    `json:"user_id"`
	Username string   `json:"username"`
	Facts    []string `json:"facts"`
}

type conversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GroupMemoryTracker tracks:
// 1. Channel conversation flow (channelID -> rolling list of messages with speaker tags e.g. "[ayushman]: ...")
// 2. Individual User profiles (userID -> distinct facts/preferences that are never mixed up)
type GroupMemoryTracker struct {
	mu             sync.Mutex
	channelHistory map[int64][]conversationMessage
	userProfiles   map[int64]*UserProfile
}

var globalGroupMemory = &GroupMemoryTracker{
	channelHistory: make(map[int64][]conversationMessage),
	userProfiles:   make(map[int64]*UserProfile),
}

// CheckCooldown checks if a user is allowed to ask or if they are on cooldown.
// Returns (canAsk, remainingSeconds).
func CheckCooldown(userID int64) (bool, int) {
	globalCooldown.mu.Lock()
	defer globalCooldown.mu.Unlock()

	last, exists := globalCooldown.lastAsks[userID]
	if !exists {
		globalCooldown.lastAsks[userID] = time.Now()
		return true, 0
	}

	elapsed := time.Since(last)
	if elapsed < CooldownDuration {
		remaining := int((CooldownDuration - elapsed).Seconds()) + 1
		return false, remaining
	}

	globalCooldown.lastAsks[userID] = time.Now()
	return true, 0
}

// ClearMemory clears the group conversation memory for a channel or user.
func ClearMemory(channelID, userID int64) {
	globalGroupMemory.mu.Lock()
	defer globalGroupMemory.mu.Unlock()

	if channelID != 0 {
		delete(globalGroupMemory.channelHistory, channelID)
	}
	if userID != 0 {
		delete(globalGroupMemory.userProfiles, userID)
	}
}

// ClearChannelMemory clears the group memory for a specific channel.
func ClearChannelMemory(channelID int64) {
	globalGroupMemory.mu.Lock()
	defer globalGroupMemory.mu.Unlock()
	delete(globalGroupMemory.channelHistory, channelID)
}

// AddMemory records a conversation turn into Spark's group channel memory and extracts user facts.
func AddMemory(channelID, userID int64, username, userPrompt, assistantAnswer string) {
	globalGroupMemory.mu.Lock()
	defer globalGroupMemory.mu.Unlock()

	key := channelID
	if key == 0 {
		key = userID
	}

	hist := globalGroupMemory.channelHistory[key]
	hist = append(hist,
		conversationMessage{Role: "user", Content: fmt.Sprintf("[%s]: %s", username, userPrompt)},
		conversationMessage{Role: "assistant", Content: assistantAnswer},
	)

	// Keep only the most recent turns
	if len(hist) > MaxChannelMemoryTurns {
		hist = hist[len(hist)-MaxChannelMemoryTurns:]
	}
	globalGroupMemory.channelHistory[key] = hist

	// Track & Update User Profile without cross-contamination
	prof, exists := globalGroupMemory.userProfiles[userID]
	if !exists {
		prof = &UserProfile{
			UserID:   userID,
			Username: username,
			Facts:    make([]string, 0),
		}
		globalGroupMemory.userProfiles[userID] = prof
	}
	prof.Username = username

	// Heuristic fact extraction for stated preferences
	extractAndAddFacts(prof, userPrompt)
}

func getChannelHistory(channelID, userID int64) []conversationMessage {
	globalGroupMemory.mu.Lock()
	defer globalGroupMemory.mu.Unlock()

	key := channelID
	if key == 0 {
		key = userID
	}

	hist := globalGroupMemory.channelHistory[key]
	result := make([]conversationMessage, len(hist))
	copy(result, hist)
	return result
}

func getUserFacts(userID int64) string {
	globalGroupMemory.mu.Lock()
	defer globalGroupMemory.mu.Unlock()

	if prof, ok := globalGroupMemory.userProfiles[userID]; ok && len(prof.Facts) > 0 {
		return strings.Join(prof.Facts, "; ")
	}
	return ""
}

// extractAndAddFacts detects explicit user preferences or background facts.
func extractAndAddFacts(prof *UserProfile, text string) {
	lower := strings.ToLower(text)
	triggers := []string{
		"i am a ", "i'm a ", "i prefer ", "my favorite ", "i love ",
		"i work as ", "i work with ", "i like ", "my hobby is ",
		"i use ", "i program in ", "i live in ", "call me ",
	}

	for _, trigger := range triggers {
		if idx := strings.Index(lower, trigger); idx != -1 {
			rawFact := strings.TrimSpace(text[idx:])
			if endIdx := strings.IndexAny(rawFact, ".\n,;!?"); endIdx != -1 && endIdx > len(trigger)+2 {
				rawFact = rawFact[:endIdx]
			}
			if len(rawFact) > 5 && len(rawFact) < 80 {
				alreadyExists := false
				for _, f := range prof.Facts {
					if strings.EqualFold(f, rawFact) {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					prof.Facts = append(prof.Facts, rawFact)
					if len(prof.Facts) > MaxUserFacts {
						prof.Facts = prof.Facts[len(prof.Facts)-MaxUserFacts:]
					}
				}
			}
		}
	}
}

// --- Google Gemini Types with Native Google Search Grounding ---

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiGoogleSearchTool struct {
	GoogleSearch struct{} `json:"google_search"`
}

type geminiRequest struct {
	SystemInstruction *geminiSystemInstruction `json:"system_instruction,omitempty"`
	Contents          []geminiContent          `json:"contents"`
	Tools             []geminiGoogleSearchTool `json:"tools,omitempty"`
	GenerationConfig  struct {
		Temperature     float64 `json:"temperature"`
		MaxOutputTokens int     `json:"maxOutputTokens"`
	} `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// cleanResponse removes accidental "@username:", "Spark:", or formatting prefixes generated by LLMs.
func cleanResponse(text, username string) string {
	text = strings.TrimSpace(text)

	prefixes := []string{
		"@" + strings.ToLower(username) + ":",
		"@" + strings.ToLower(username) + ",",
		"@" + strings.ToLower(username),
		"[" + strings.ToLower(username) + "]:",
		"spark:",
		"assistant:",
		"ai:",
	}

	for {
		changed := false
		lower := strings.ToLower(text)
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				text = strings.TrimSpace(text[len(p):])
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}

	return text
}

// AskAI routes prompts to Google Gemini with Live Search Grounding.
func AskAI(ctx context.Context, cfg AIConfig, channelID, userID int64, username, prompt string) (string, error) {
	promptTrimmed := strings.TrimSpace(prompt)
	lowerPrompt := strings.ToLower(promptTrimmed)

	// Support memory reset commands
	if lowerPrompt == "clear" || lowerPrompt == "reset" || lowerPrompt == "forget" || lowerPrompt == "clear memory" {
		ClearMemory(channelID, userID)
		return "🧹 I've cleared the conversation memory for this channel! What would you like to talk about next?", nil
	}
	if lowerPrompt == "forget me" {
		ClearMemory(0, userID)
		return fmt.Sprintf("🧹 I've cleared all personal preferences and notes for @%s!", username), nil
	}

	if cfg.GeminiAPIKey == "" {
		return "⚠️ Gemini API key is not configured on this server.\nSet `GEMINI_API_KEY` in your `.env` file to enable Spark (🤖).", nil
	}

	now := time.Now()
	dateStr := now.Format("Monday, 02 Jan 2006")

	userFacts := getUserFacts(userID)
	userFactSection := ""
	if userFacts != "" {
		userFactSection = fmt.Sprintf("\n- User preferences for @%s: %s", username, userFacts)
	}

	// Perform real-time web search grounding (like Meta AI)
	liveWebContext := SearchLiveWeb(ctx, promptTrimmed)

	systemPrompt := fmt.Sprintf(
		"You are Spark (🤖), an intelligent, real-time AI assistant in Shell Chat.\n"+
			"Current Date: %s (Year %d).\n"+
			"User: @%s.\n\n"+
			"Directives:\n"+
			"1. Respond directly, naturally, and helpfully to the user's inquiry.\n"+
			"2. DO NOT prefix or start your response with '@%s:' or '@%s,' or 'Spark:'. Begin immediately with your helpful answer.\n"+
			"3. Real-Time Knowledge & Grounding: You have access to real-time live web search results below. Always use the live search information to answer questions about recent events (2024-2026), current leadership, news, and world facts accurately.\n"+
			"4. Group Conversation Continuity: If following up on an earlier discussion in this channel, respond naturally in context.\n"+
			"5. User Preferences: Keep personal preferences strictly tied to this user.%s%s\n"+
			"6. Format cleanly for terminal display (typically 2-4 concise sentences or short bullet points).",
		dateStr, now.Year(), username, username, username, userFactSection, liveWebContext,
	)

	modelsToTry := []string{
		cfg.GeminiModel,
		"gemini-1.5-flash",
		"gemini-1.5-flash-latest",
		"gemini-1.5-pro",
		"gemini-2.5-flash",
	}

	for _, m := range modelsToTry {
		if m == "" {
			continue
		}
		answer, err := askGemini(ctx, cfg.GeminiAPIKey, m, systemPrompt, channelID, userID, username, promptTrimmed)
		if err == nil && answer != "" {
			answer = cleanResponse(answer, username)
			AddMemory(channelID, userID, username, promptTrimmed, answer)
			return answer, nil
		}
	}

	// Clean standard issue message
	return "🤖 Spark is momentarily unavailable. Please try again in a few moments.", nil
}

// askGemini queries Google Gemini with native Google Search Grounding enabled.
func askGemini(ctx context.Context, apiKey, model, systemPrompt string, channelID, userID int64, username, prompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	var contents []geminiContent

	// Add channel history
	pastTurns := getChannelHistory(channelID, userID)
	for _, m := range pastTurns {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}

	// Add current prompt
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: prompt}},
	})

	// 1. Try with Google Search Grounding tool
	reqBodyWithSearch := geminiRequest{
		SystemInstruction: &geminiSystemInstruction{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: contents,
		Tools:    []geminiGoogleSearchTool{{}},
	}
	reqBodyWithSearch.GenerationConfig.Temperature = 0.4
	reqBodyWithSearch.GenerationConfig.MaxOutputTokens = 500

	ans, err := executeGeminiReq(ctx, url, reqBodyWithSearch)
	if err == nil && ans != "" {
		return ans, nil
	}

	// 2. If tools rejected on this tier, try standard generation
	reqBodyStandard := geminiRequest{
		SystemInstruction: &geminiSystemInstruction{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: contents,
	}
	reqBodyStandard.GenerationConfig.Temperature = 0.4
	reqBodyStandard.GenerationConfig.MaxOutputTokens = 500

	return executeGeminiReq(ctx, url, reqBodyStandard)
}

func executeGeminiReq(ctx context.Context, url string, reqBody geminiRequest) (string, error) {
	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return "", err
	}

	if geminiResp.Error != nil && geminiResp.Error.Message != "" {
		return "", fmt.Errorf("gemini error: %s", geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text), nil
	}

	return "", fmt.Errorf("empty response from Gemini")
}
