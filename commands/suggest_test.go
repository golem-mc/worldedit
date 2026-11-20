package commands

import (
	"slices"
	"testing"
)

func TestSuggestPattern(t *testing.T) {
	got := suggestPattern(nil, "st")
	if !slices.ContainsFunc(got, func(s string) bool { return s == "minecraft:stone" || s == "stone" }) {
		t.Fatalf("suggestions for %q lack stone: %v", "st", got)
	}
	got = suggestPattern(nil, "50%stone,dirt")
	for _, s := range got {
		if len(s) < 7 || s[:7] != "50%stone,"[:7] {
			t.Fatalf("suggestion %q lost its prefix", s)
		}
	}
	if len(got) == 0 {
		t.Fatal("no suggestions after a comma list")
	}
}
