package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// transcriptRecord is the minimal shape of a Claude Code JSONL line.
type transcriptRecord struct {
	Type    string `json:"type"`
	AiTitle string `json:"aiTitle"`
	Message *struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

var (
	fenceRe = regexp.MustCompile("(?s)```[^`]*?```")
)

var wsRe = regexp.MustCompile(`[ \t]+`)

// lastSentenceFromText strips basic markdown from text, collapses whitespace,
// then returns the last ". "-delimited segment.
func lastSentenceFromText(text string) string {
	// Drop fenced code blocks.
	text = fenceRe.ReplaceAllString(text, "")

	// Strip backtick inline code markers.
	text = strings.ReplaceAll(text, "`", "")

	// Normalize line endings and split into lines for per-line processing.
	// Treat each newline as a potential sentence boundary by joining kept lines
	// with ". " when the previous line does not already end with punctuation.
	rawLines := strings.Split(text, "\n")
	var kept []string
	for _, line := range rawLines {
		// Collapse horizontal whitespace within each line.
		line = strings.TrimSpace(wsRe.ReplaceAllString(line, " "))
		if line == "" {
			continue
		}
		// Drop lines that are purely markdown headings or list markers.
		if strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "- ") ||
			strings.HasPrefix(line, "* ") {
			continue
		}
		kept = append(kept, line)
	}

	if len(kept) == 0 {
		return ""
	}

	// Join lines: use ". " as separator so newline boundaries become sentence
	// splits, unless the preceding line already ends with sentence-ending punctuation.
	var sb strings.Builder
	for i, line := range kept {
		if i == 0 {
			sb.WriteString(line)
			continue
		}
		prev := kept[i-1]
		last := prev[len(prev)-1]
		if last == '.' || last == '!' || last == '?' {
			sb.WriteString(" ")
		} else {
			sb.WriteString(". ")
		}
		sb.WriteString(line)
	}
	text = sb.String()

	// Split on ". " and take the last non-empty segment.
	parts := strings.Split(text, ". ")
	var last string
	for i := len(parts) - 1; i >= 0; i-- {
		seg := strings.TrimSpace(parts[i])
		if seg != "" {
			last = seg
			break
		}
	}

	return last
}

// looksLikeCode returns true when the segment appears to be a code token
// rather than natural-language prose.
func looksLikeCode(s string) bool {
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "}") {
		return true
	}
	if !strings.Contains(s, " ") && len(s) > 40 {
		return true
	}
	return false
}

// LastSentence reads the Claude Code JSONL transcript at transcriptPath and
// returns the last sentence of the agent's last assistant message. It falls
// back to the last seen aiTitle when the extracted sentence looks like code or
// is empty. Returns "" when the file cannot be read.
func LastSentence(transcriptPath string) string {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastAssistantText string
	var lastAiTitle string

	scanner := bufio.NewScanner(f)
	// Increase buffer for potentially long lines.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var rec transcriptRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.AiTitle != "" {
			lastAiTitle = rec.AiTitle
		}
		if rec.Type == "assistant" && rec.Message != nil {
			for _, block := range rec.Message.Content {
				if block.Type == "text" && block.Text != "" {
					lastAssistantText = block.Text
					break
				}
			}
		}
	}

	if lastAssistantText != "" {
		sentence := lastSentenceFromText(lastAssistantText)
		if sentence != "" && !looksLikeCode(sentence) {
			return sentence
		}
	}

	return lastAiTitle
}
