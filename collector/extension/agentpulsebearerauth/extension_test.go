package agentpulsebearerauth

import (
	"strings"
	"testing"
)

func TestExtractBearer(t *testing.T) {
	cases := []struct {
		name    string
		sources map[string][]string
		want    string
	}{
		{
			name:    "lowercased key with Bearer prefix",
			sources: map[string][]string{"authorization": {"Bearer abc123"}},
			want:    "abc123",
		},
		{
			name:    "title-cased key",
			sources: map[string][]string{"Authorization": {"Bearer xyz"}},
			want:    "xyz",
		},
		{
			name:    "lowercase bearer prefix",
			sources: map[string][]string{"authorization": {"bearer foo"}},
			want:    "foo",
		},
		{
			name:    "missing header",
			sources: map[string][]string{},
			want:    "",
		},
		{
			name:    "wrong scheme",
			sources: map[string][]string{"authorization": {"Basic xxx"}},
			want:    "",
		},
		{
			name:    "empty bearer value",
			sources: map[string][]string{"authorization": {"Bearer "}},
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractBearer(tc.sources)
			if got != tc.want {
				t.Errorf("extractBearer = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHashTokenIsStableSHA256Hex(t *testing.T) {
	h := hashToken("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if h != want {
		t.Fatalf("hashToken(hello) = %s, want %s", h, want)
	}
	if len(h) != 64 || strings.ContainsAny(h, "ghijklmnopqrstuvwxyz") {
		t.Errorf("expected lowercase hex output, got %q", h)
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (&Config{}).Validate(); err == nil {
		t.Error("expected error for empty DSN")
	}
	if err := (&Config{DSN: "postgres://x"}).Validate(); err != nil {
		t.Errorf("unexpected error for valid DSN: %v", err)
	}
}

func TestFromContext(t *testing.T) {
	ctx := contextWithAuth(t.Context(), AuthInfo{ProjectID: "proj-1", TokenID: "tok-1"})
	info, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected AuthInfo in ctx")
	}
	if info.ProjectID != "proj-1" || info.TokenID != "tok-1" {
		t.Errorf("got %+v", info)
	}

	if _, ok := FromContext(t.Context()); ok {
		t.Error("expected no AuthInfo on bare context")
	}
}
