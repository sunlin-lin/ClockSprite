package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const firstNonsenseQuote = 200
const maxQuoteFile = 1 << 20

func parseQuotes(text string) ([]string, error) {
	if len(text) > maxQuoteFile || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return nil, errors.New("請使用 1 MB 以下的 UTF-8 文字檔。")
	}
	lines := []string{}
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimPrefix(text, "\uFEFF"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			lines = append(lines, line)
			seen[line] = true
		}
	}
	return lines, nil
}

var nonsenseQuotes []string
var customQuotes bool

func quotePath() string { return filepath.Join(filepath.Dir(settingsPath), "nonsense-quotes.json") }

func loadQuotes() error {
	raw, err := resources.ReadFile("quotes/nonsense.txt")
	if err != nil {
		return err
	}
	nonsenseQuotes, err = parseQuotes(string(raw))
	if err != nil {
		return err
	}
	customQuotes = false
	var saved []string
	found, err := readStored(quotePath(), &saved)
	if err == nil && found {
		nonsenseQuotes, customQuotes = saved, true
	}
	return err
}

func speechCount() int { return firstNonsenseQuote + len(nonsenseQuotes) }

func customSpeech(pick int) string {
	if customQuotes && pick >= firstNonsenseQuote && pick < speechCount() {
		return nonsenseQuotes[pick-firstNonsenseQuote]
	}
	return ""
}

func handleQuoteAction(id int) {
	switch id {
	case actionQuoteSettings:
		showQuoteEditor(strings.Join(nonsenseQuotes, "\n"))
	case actionQuoteSave:
		lines, err := parseQuotes(quoteEditorText())
		if err == nil {
			err = writeJSON(quotePath(), lines)
		}
		if err != nil {
			notice(err.Error())
			return
		}
		nonsenseQuotes, customQuotes = lines, true
		if lastQuote >= firstNonsenseQuote {
			lastQuote = -1
			if activeReminder.action == "" {
				paintMascot(-1)
			}
		}
		closeQuoteEditor()
	case actionQuoteImport:
		if path := quoteFile(false); path != "" {
			err := importQuotes(path)
			if err != nil {
				notice(err.Error())
			}
		}
	case actionQuoteExport:
		lines, err := parseQuotes(quoteEditorText())
		if err == nil {
			if path := quoteFile(true); path != "" {
				content := strings.Join(lines, "\n")
				if len(lines) > 0 {
					content += "\n"
				}
				err = os.WriteFile(path, []byte(content), 0600)
			}
		}
		if err != nil {
			notice(err.Error())
		}
	}
}

func importQuotes(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxQuoteFile {
		return errors.New("請選擇 1 MB 以下的文字檔。")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	existing, err := parseQuotes(quoteEditorText())
	if err != nil {
		return err
	}
	added, err := parseQuotes(string(raw))
	if err != nil {
		return err
	}
	merged, err := parseQuotes(strings.Join(append(existing, added...), "\n"))
	if err != nil {
		return err
	}
	if len(existing) == len(merged) {
		return errors.New("沒有可加入的新語錄。")
	}
	setQuoteEditorText(strings.Join(merged, "\n"))
	return nil
}
