package utils

import (
	"strings"
	"unicode"
)

// JaccardSimilarity returns the Jaccard index of two strings (token set overlap).
func JaccardSimilarity(a, b string) float64 {
	tokensA := tokenize(a)
	tokensB := tokenize(b)
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}
	intersect := 0
	for t := range tokensA {
		if tokensB[t] {
			intersect++
		}
	}
	union := len(tokensA) + len(tokensB) - intersect
	return float64(intersect) / float64(union)
}

func tokenize(s string) map[string]bool {
	tokens := make(map[string]bool)
	for _, word := range strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}) {
		tokens[strings.ToLower(word)] = true
	}
	return tokens
}
