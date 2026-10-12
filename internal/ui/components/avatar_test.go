package components

import (
	"reflect"
	"strings"
	"testing"
)

func TestAvatarUsesDistinctPixelSprites(t *testing.T) {
	claude := Avatar("claude", "", "")
	codex := Avatar("codex", "", "")
	if len(claude) != 4 || len(codex) != 4 {
		t.Fatalf("avatar rows claude=%d codex=%d, want 4", len(claude), len(codex))
	}
	if reflect.DeepEqual(claude, codex) {
		t.Fatal("provider avatars should have distinct silhouettes")
	}
	if !strings.Contains(strings.Join(claude, ""), "▀") {
		t.Fatal("pixel avatar contains no half-block pixels")
	}
}

func TestAvatarCustomSymbolFallback(t *testing.T) {
	got := Avatar("custom", "#FF00AA", "✦")
	if len(got) != 4 || !strings.Contains(got[0], "✦") {
		t.Fatalf("custom avatar=%q", got)
	}
	for _, row := range got[1:] {
		if row != "" {
			t.Fatalf("unexpected custom avatar row %q", row)
		}
	}
}

func TestPixelAvatarPresetsRenderDistinctSprites(t *testing.T) {
	first := Avatar("custom", "#B34CFF", "pixel1")
	square := Avatar("custom", "#714BFF", "pixel8")
	if len(first) != 4 || len(square) != 4 {
		t.Fatalf("preset rows first=%d square=%d, want 4", len(first), len(square))
	}
	if reflect.DeepEqual(first, square) {
		t.Fatal("pixel avatar presets should have distinct silhouettes")
	}
	if !strings.Contains(strings.Join(first, ""), "▀") || !strings.Contains(strings.Join(square, ""), "▀") {
		t.Fatal("pixel avatar presets did not render half-block pixels")
	}
}
