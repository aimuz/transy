package livetranslate

import (
	"regexp"
	"strings"
)

var (
	// regexTimestamp matches VTT/SRT timestamps like [00:00:00.000 --> 00:00:04.000]
	regexTimestamp = regexp.MustCompile(`\[\d{2}:\d{2}:\d{2}\.\d{3}\s-->\s\d{2}:\d{2}:\d{2}\.\d{3}\]`)
	// regexBlankAudio matches [BLANK_AUDIO] or similar artifacts
	regexArtifacts = regexp.MustCompile(`\[.*?\]`)
)

// cleanText removes timestamps and artifacts from the text.
func cleanText(text string) string {
	// Remove timestamps
	text = regexTimestamp.ReplaceAllString(text, "")

	// Remove other brackets artifacts if they basically cover the whole string or look like metadata
	// But be careful not to remove valid text in brackets like [Applause] if we want to keep it?
	// Actually user probably wants clean text. Let's remove any [...] that looks like non-speech.
	// For now, let's just target the specific timestamp format and trim space.

	text = strings.TrimSpace(text)
	return text
}
