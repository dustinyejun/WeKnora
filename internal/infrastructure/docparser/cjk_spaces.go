package docparser

import (
	"strings"
	"unicode"
)

// JoinCJKSpaces removes spaces that a parser left between two CJK characters.
// PDF extraction often turns layout gaps and line wraps into spaces
// ("嵌入单 位信息系统", "归 口管理"), which breaks keyword search on the
// stored text. Newlines are kept, so Markdown structure is untouched, and the
// space after a clause number ("第十八条 单位应当", "第一章 总则") is kept
// because it separates the number from the text.
//
// Source-block remapping compares letters and digits only, so removing spaces
// here does not shift citation locations.
func JoinCJKSpaces(text string) string {
	if text == "" {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if !isInlineSpace(r) {
			b.WriteRune(r)
			continue
		}
		j := i
		for j < len(runes) && isInlineSpace(runes[j]) {
			j++
		}
		if i > 0 && j < len(runes) && isCJK(runes[i-1]) && isCJK(runes[j]) && !endsWithClauseNumber(runes[:i]) {
			i = j - 1
			continue
		}
		for k := i; k < j; k++ {
			b.WriteRune(runes[k])
		}
		i = j - 1
	}
	return b.String()
}

func isInlineSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\u00a0' || r == '\u3000'
}

// isCJK covers Han characters, CJK punctuation and full-width forms.
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3001 && r <= 0x303f) ||
		(r >= 0xff01 && r <= 0xff60)
}

// endsWithClauseNumber reports whether text ends in 第…章/节/条/款/项/编,
// e.g. "第十八条" or "第3章".
func endsWithClauseNumber(text []rune) bool {
	n := len(text)
	if n < 3 || !strings.ContainsRune("章节条款项编", text[n-1]) {
		return false
	}
	i := n - 2
	for i >= 0 && (strings.ContainsRune("一二三四五六七八九十百千零〇两", text[i]) || (text[i] >= '0' && text[i] <= '9')) {
		i--
	}
	return i >= 0 && i < n-2 && text[i] == '第'
}

// JoinCJKLineWraps joins PDF line wraps that split a Chinese sentence
// ("嵌入单\n位信息系统"). A single newline is removed only when both sides are
// Han characters, the previous line is not a Markdown heading or table row,
// and the next line does not start a new clause ("第十九条"). Sentence-ending
// punctuation, list markers and blank lines keep their breaks, so paragraphs
// and lists survive. Meant for PDF text only: in Word or Markdown sources a
// line break between two characters is usually deliberate.
func JoinCJKLineWraps(text string) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text))
	lineStart := 0
	for i, r := range runes {
		if r != '\n' {
			b.WriteRune(r)
			continue
		}
		if i > 0 && i+1 < len(runes) &&
			unicode.Is(unicode.Han, runes[i-1]) && unicode.Is(unicode.Han, runes[i+1]) &&
			!startsStructuralLine(runes[lineStart:i]) && !startsClause(runes[i+1:]) {
			continue
		}
		b.WriteRune(r)
		lineStart = i + 1
	}
	return b.String()
}

func startsStructuralLine(line []rune) bool {
	s := strings.TrimSpace(string(line))
	return strings.HasPrefix(s, "#") || strings.HasPrefix(s, "|")
}

// startsClause reports whether text begins with 第…章/节/条/款/项/编.
func startsClause(text []rune) bool {
	if len(text) < 3 || text[0] != '第' {
		return false
	}
	i := 1
	for i < len(text) && (strings.ContainsRune("一二三四五六七八九十百千零〇两", text[i]) || (text[i] >= '0' && text[i] <= '9')) {
		i++
	}
	return i > 1 && i < len(text) && strings.ContainsRune("章节条款项编", text[i])
}
