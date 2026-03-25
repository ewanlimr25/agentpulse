package eval

import "testing"

// ── stripMarkdownFences ───────────────────────────────────────────────────────

func TestStripMarkdownFencesPlain(t *testing.T) {
	in := `{"score":0.8,"reasoning":"ok"}`
	got := stripMarkdownFences(in)
	if got != in {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestStripMarkdownFencesJsonFence(t *testing.T) {
	in := "```json\n{\"score\":0.8,\"reasoning\":\"ok\"}\n```"
	got := stripMarkdownFences(in)
	want := `{"score":0.8,"reasoning":"ok"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripMarkdownFencesGenericFence(t *testing.T) {
	in := "```\n{\"score\":0.5,\"reasoning\":\"meh\"}\n```"
	got := stripMarkdownFences(in)
	want := `{"score":0.5,"reasoning":"meh"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripMarkdownFencesWhitespace(t *testing.T) {
	in := "  ```json\n{}\n```  "
	got := stripMarkdownFences(in)
	if got != "{}" {
		t.Errorf("got %q, want {}", got)
	}
}
