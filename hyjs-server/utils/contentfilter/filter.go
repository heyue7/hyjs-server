// Package contentfilter checks the embedded, versioned sexual/political lexicons.
package contentfilter

import (
	"embed"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

//go:embed data/*.txt
var data embed.FS

type node struct {
	next     map[rune]*node
	end      bool
	boundary bool
}

type matcher struct {
	root        *node
	traditional map[rune]rune
}

// The immutable matcher is built once, with no request-time files or networking.
var defaultMatcher = load()

func load() *matcher {
	m := &matcher{root: &node{next: make(map[rune]*node)}, traditional: make(map[rune]rune)}
	raw, err := data.ReadFile("data/TSCharacters.txt")
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 || strings.HasPrefix(parts[0], "#") {
			continue
		}
		from, to := []rune(parts[0]), []rune(parts[1])
		if len(from) == 1 && len(to) == 1 {
			m.traditional[from[0]] = to[0]
		}
	}
	excluded := make(map[string]bool)
	raw, err = data.ReadFile("data/exclusions.txt")
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		excluded[strings.TrimSpace(line)] = true
	}
	for _, filename := range []string{"政治类型.txt", "反动词库.txt", "色情类型.txt", "色情词库.txt", "supplement.txt"} {
		raw, err = data.ReadFile("data/" + filename)
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			entry := strings.TrimPrefix(strings.TrimSpace(line), "\ufeff")
			if excluded[entry] {
				continue
			}
			word := m.normalize(entry)
			// Reject broad single-character entries by default. Explicit sexual
			// terms remain blocked, including standalone names made from them.
			if len(word) == 0 || len(word) == 1 && !strings.ContainsRune("屄屌肏", word[0]) {
				continue
			}
			n := m.root
			for _, r := range word {
				if n.next[r] == nil {
					n.next[r] = &node{next: make(map[rune]*node)}
				}
				n = n.next[r]
			}
			n.end = true
			// Ambiguous Latin roots must stand alone, preserving words such as
			// small and analysis. Other entries retain substring matching.
			n.boundary = string(word) == "sm" || string(word) == "sb" || string(word) == "anal"
		}
	}
	return m
}

func (m *matcher) normalize(text string) []rune {
	var out []rune
	for _, r := range norm.NFKC.String(text) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		r = unicode.ToLower(r)
		if simple, ok := m.traditional[r]; ok {
			r = simple
		}
		out = append(out, r)
	}
	return out
}

// Contains ignores case, compatibility width, separators, symbols and invisible
// formatting. Traditional character variants use OpenCC's TSCharacters mapping.
// Matching never alters the accepted text and does not perform semantic analysis.
func Contains(text string) bool {
	return defaultMatcher.contains(text)
}

func (m *matcher) contains(text string) bool {
	original := []rune(norm.NFKC.String(text))
	var textRunes []rune
	var positions []int
	for i, r := range original {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		r = unicode.ToLower(r)
		if simple, ok := m.traditional[r]; ok {
			r = simple
		}
		textRunes = append(textRunes, r)
		positions = append(positions, i)
	}
	for i := range textRunes {
		n := m.root
		for offset, r := range textRunes[i:] {
			n = n.next[r]
			if n == nil {
				break
			}
			if n.end && (!n.boundary || wordBoundary(original, positions[i]-1) && wordBoundary(original, positions[i+offset]+1)) {
				return true
			}
		}
	}
	return false
}

func wordBoundary(text []rune, i int) bool {
	if i < 0 || i >= len(text) {
		return true
	}
	r := unicode.ToLower(text[i])
	return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
}
