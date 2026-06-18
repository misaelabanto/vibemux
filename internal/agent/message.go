package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"
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

// transcriptScan holds the data extracted from a single pass over a transcript.
type transcriptScan struct {
	// lastText is the most recent non-empty assistant text block.
	lastText string
	// lastAiTitle is the most recent aiTitle record.
	lastAiTitle string
	// endsWithText reports whether the turn looks complete: the last assistant
	// text block is not followed by a later tool_use block. Claude Code can fire
	// the Stop hook before the final assistant text is flushed to disk, in which
	// case the last assistant activity is still a tool_use and this is false.
	endsWithText bool
}

// scanTranscript does one pass over the Claude Code JSONL transcript.
func scanTranscript(transcriptPath string) transcriptScan {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return transcriptScan{}
	}
	defer f.Close()

	var res transcriptScan
	toolUseAfterText := false

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
			res.lastAiTitle = rec.AiTitle
		}
		if rec.Type == "assistant" && rec.Message != nil {
			for _, block := range rec.Message.Content {
				switch {
				case block.Type == "text" && block.Text != "":
					res.lastText = block.Text
					toolUseAfterText = false
				case block.Type == "tool_use":
					toolUseAfterText = true
				}
			}
		}
	}

	res.endsWithText = res.lastText != "" && !toolUseAfterText
	return res
}

// sentenceFrom derives the display sentence from a scan, falling back to the
// aiTitle when the extracted sentence is empty or looks like code.
func sentenceFrom(s transcriptScan) string {
	if s.lastText != "" {
		sentence := lastSentenceFromText(s.lastText)
		if sentence != "" && !looksLikeCode(sentence) {
			return sentence
		}
	}
	return s.lastAiTitle
}

// LastSentence reads the Claude Code JSONL transcript at transcriptPath and
// returns the last sentence of the agent's last assistant message. It falls
// back to the last seen aiTitle when the extracted sentence looks like code or
// is empty. Returns "" when the file cannot be read.
func LastSentence(transcriptPath string) string {
	return sentenceFrom(scanTranscript(transcriptPath))
}

// transcript settle tuning: Claude Code may invoke the Stop hook a few dozen ms
// before the final assistant text block is flushed to the transcript file.
var (
	transcriptSettleTimeout = 500 * time.Millisecond
	transcriptSettleStep    = 25 * time.Millisecond
)

// LastSentenceFinal is like LastSentence but tolerant of the Stop-hook flush
// race: it polls the transcript until the last assistant block is text (a
// completed turn) or a short timeout elapses, then extracts the sentence. Use
// this from the Stop handler so a mid-turn preamble is not captured as the
// final message.
func LastSentenceFinal(transcriptPath string) string {
	deadline := time.Now().Add(transcriptSettleTimeout)
	scan := scanTranscript(transcriptPath)
	for !scan.endsWithText && time.Now().Before(deadline) {
		time.Sleep(transcriptSettleStep)
		scan = scanTranscript(transcriptPath)
	}
	return sentenceFrom(scan)
}
