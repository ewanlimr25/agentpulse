package piimaskerproc

import (
	"regexp"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// helper: build patterns + combined regex from builtins.
func testBuiltins() ([]piiPattern, *regexp.Regexp) {
	p := builtinPatterns()
	return p, buildCombinedRegex(p)
}

// ── Individual pattern tests ────────────────────────────────────────────────

func TestCreditCard_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"card number 4111111111111111 on file",
		"5500005555555559",
		"378282246310005 is the amex card",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("credit_card: expected match in %q", s)
		}
	}
}

func TestCreditCard_NoMatch(t *testing.T) {
	patterns, combined := testBuiltins()
	negatives := []string{
		"trace id 1a2b3c4d5e6f7a8b",            // hex trace ID, not all decimal
		"order number 12345678901234567890",       // too many digits, no word boundary match for standard CC lengths
	}
	for _, s := range negatives {
		redacted, count := redact(s, patterns, combined)
		// Only flag if count > 0 for the credit_card pattern specifically
		_ = redacted
		// The combined regex may still fire for other patterns; check specifically
		for _, p := range patterns {
			if p.name == "credit_card" && p.re.MatchString(s) {
				t.Errorf("credit_card: unexpected match in %q", s)
			}
		}
		_ = count
	}
}

func TestSSN_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"SSN: 123-45-6789",
		"social security 987-65-4321 was provided",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("ssn: expected match in %q", s)
		}
	}
}

func TestSSN_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	negatives := []string{
		"phone 123-456-7890",   // phone, not SSN format
		"id 12-34-5678",        // wrong grouping
	}
	for _, s := range negatives {
		for _, p := range patterns {
			if p.name == "ssn" && p.re.MatchString(s) {
				t.Errorf("ssn: unexpected match in %q", s)
			}
		}
	}
}

func TestEmail_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"user alice@example.com logged in",
		"contact support@agentpulse.io for help",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("email: expected match in %q", s)
		}
	}
}

func TestEmail_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	negatives := []string{
		"not-an-email",
		"missing@tld",
	}
	for _, s := range negatives {
		for _, p := range patterns {
			if p.name == "email" && p.re.MatchString(s) {
				t.Errorf("email: unexpected match in %q", s)
			}
		}
	}
}

func TestJWT_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	positives := []string{
		"token: " + jwt,
		jwt + " was used",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("jwt: expected match in %q", s)
		}
	}
}

func TestJWT_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	negatives := []string{
		"eyJhbGc.short.abc",           // parts too short
		"notaJWT.atAll.here",          // doesn't start with eyJ
	}
	for _, s := range negatives {
		for _, p := range patterns {
			if p.name == "jwt" && p.re.MatchString(s) {
				t.Errorf("jwt: unexpected match in %q", s)
			}
		}
	}
}

func TestOpenAIKey_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"key sk-abcdefghijklmnopqrstu is set",
		"OPENAI_API_KEY=sk-ABCDEFGHIJKLMNOPQRSTUV",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("api_key_openai: expected match in %q", s)
		}
	}
}

func TestOpenAIKey_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	negatives := []string{
		"sk-short",         // too short
		"sk_not_openai",    // wrong prefix format
	}
	for _, s := range negatives {
		for _, p := range patterns {
			if p.name == "api_key_openai" && p.re.MatchString(s) {
				t.Errorf("api_key_openai: unexpected match in %q", s)
			}
		}
	}
}

func TestAnthropicKey_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"key sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZ01234567 is set",
		"sk-ant-admin01-abcdefghijklmnopqrstuvwxyz0123456789abcdefgh",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("api_key_anthropic: expected match in %q", s)
		}
	}
}

func TestAnthropicKey_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	negatives := []string{
		"sk-ant-short",   // too short
		"sk-openai-key",  // wrong prefix
	}
	for _, s := range negatives {
		for _, p := range patterns {
			if p.name == "api_key_anthropic" && p.re.MatchString(s) {
				t.Errorf("api_key_anthropic: unexpected match in %q", s)
			}
		}
	}
}

func TestBearerInline_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"Authorization: Bearer eyJhbGciOiJSUzI1NiJ9.abc.def",
		"header Bearer abcdefghijklmnop was sent",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("bearer_inline: expected match in %q", s)
		}
	}
}

func TestAWSAccessKey_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"access key AKIAIOSFODNN7EXAMPLE is exposed",
		"AKIAI44QH8DHBEXAMPLE",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("aws_access_key: expected match in %q", s)
		}
	}
}

func TestAWSAccessKey_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	negatives := []string{
		"AKIA_lowercase_invalid",
		"BKIAIOSFODNN7EXAMPLE",  // wrong prefix
	}
	for _, s := range negatives {
		for _, p := range patterns {
			if p.name == "aws_access_key" && p.re.MatchString(s) {
				t.Errorf("aws_access_key: unexpected match in %q", s)
			}
		}
	}
}

func TestGitHubPAT_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		// ghp_ followed by exactly 36 alphanumeric chars (A-Z=26 + 0-9=10)
		"token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("github_pat: expected match in %q", s)
		}
	}
}

func TestGitHubPAT_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	negatives := []string{
		"ghp_tooshort",
		"github_pat_not_right",
	}
	for _, s := range negatives {
		for _, p := range patterns {
			if p.name == "github_pat" && p.re.MatchString(s) {
				t.Errorf("github_pat: unexpected match in %q", s)
			}
		}
	}
}

func TestStripeKey_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"sk_live_" + "abcdefghijklmnopqrstuvwx",
		"key sk_live_" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ01 set",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("stripe_live: expected match in %q", s)
		}
	}
}

func TestGoogleAPIKey_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		// AIza followed by exactly 35 chars matching [0-9A-Za-z_-]
		"key AIzaSyDabcdefghijklmnopqrstuvwxyz012345",
		"AIzaSyBabcdefghijklmnopqrstuvwxyz012345",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("google_api_key: expected match in %q", s)
		}
	}
}

func TestPEMHeader_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...",
		"key: -----BEGIN PRIVATE KEY----- data",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("pem_header: expected match in %q", s)
		}
	}
}

func TestSlackToken_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"token xoxb-abcdefghij-klmnopqrstu",
		"xoxp-123456789-abcdefghijklm",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("slack_token: expected match in %q", s)
		}
	}
}

func TestPhoneUS_WithAreaCode_Matches(t *testing.T) {
	patterns, combined := testBuiltins()
	positives := []string{
		"call (555) 867-5309 now",
		"phone: 555.867.5309",
		"+1 555-867-5309",
	}
	for _, s := range positives {
		_, count := redact(s, patterns, combined)
		if count == 0 {
			t.Errorf("phone_us: expected match in %q", s)
		}
	}
}

func TestPhoneUS_WithoutAreaCode_NoMatch(t *testing.T) {
	patterns, _ := testBuiltins()
	// 7-digit number (no area code) should NOT match.
	for _, p := range patterns {
		if p.name == "phone_us" && p.re.MatchString("555-1234") {
			t.Error("phone_us: should not match 7-digit number without area code")
		}
	}
}

// ── redact() function tests ──────────────────────────────────────────────────

func TestRedact_NoMatch_ReturnsOriginal(t *testing.T) {
	patterns, combined := testBuiltins()
	input := "no PII here at all"
	got, count := redact(input, patterns, combined)
	if got != input {
		t.Errorf("expected original string, got %q", got)
	}
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}
}

func TestRedact_EmptyString(t *testing.T) {
	patterns, combined := testBuiltins()
	got, count := redact("", patterns, combined)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}
}

func TestRedact_MultiplePatterns_OneString(t *testing.T) {
	patterns, combined := testBuiltins()
	input := "user alice@example.com has SSN 123-45-6789 and card 4111111111111111"
	got, count := redact(input, patterns, combined)
	if count < 3 {
		t.Errorf("expected at least 3 redactions, got %d in %q", count, got)
	}
	if strings.Contains(got, "@") {
		t.Errorf("email should be redacted, got %q", got)
	}
	if strings.Contains(got, "123-45-6789") {
		t.Errorf("SSN should be redacted, got %q", got)
	}
	if strings.Contains(got, "4111111111111111") {
		t.Errorf("credit card should be redacted, got %q", got)
	}
}

func TestRedact_EarlyExitViaCombinedRegex(t *testing.T) {
	patterns, combined := testBuiltins()
	// A string with no PII. The combined regex should not match.
	input := "clean log message: processed 42 items successfully"
	got, count := redact(input, patterns, combined)
	if got != input || count != 0 {
		t.Errorf("expected no-op redaction, got count=%d value=%q", count, got)
	}
}

func TestRedact_ReplacementFormat(t *testing.T) {
	patterns, combined := testBuiltins()
	input := "email: alice@example.com"
	got, _ := redact(input, patterns, combined)
	if !strings.Contains(got, "[REDACTED:email]") {
		t.Errorf("expected [REDACTED:email] token, got %q", got)
	}
}

// ── parseCustomRules tests ────────────────────────────────────────────────────

func TestParseCustomRules_ValidRule(t *testing.T) {
	logger := zaptest.NewLogger(t)
	rules := []piiCustomRule{
		{Name: "account_id", Pattern: `\bACCT-\d{8}\b`, Enabled: true},
	}
	got := parseCustomRules(logger, rules)
	if len(got) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(got))
	}
	if got[0].name != "account_id" {
		t.Errorf("expected name=account_id, got %q", got[0].name)
	}
}

func TestParseCustomRules_InvalidRegex_Skipped(t *testing.T) {
	logger := zaptest.NewLogger(t)
	rules := []piiCustomRule{
		{Name: "bad", Pattern: `[invalid`, Enabled: true},
	}
	got := parseCustomRules(logger, rules)
	if len(got) != 0 {
		t.Errorf("expected invalid regex to be skipped, got %d patterns", len(got))
	}
}

func TestParseCustomRules_DisabledRule_Skipped(t *testing.T) {
	logger := zaptest.NewLogger(t)
	rules := []piiCustomRule{
		{Name: "off", Pattern: `\bsecret\b`, Enabled: false},
	}
	got := parseCustomRules(logger, rules)
	if len(got) != 0 {
		t.Errorf("expected disabled rule to be skipped, got %d patterns", len(got))
	}
}

func TestParseCustomRules_EmptyMatchPattern_Skipped(t *testing.T) {
	logger := zaptest.NewLogger(t)
	// Pattern ".*" matches empty string — must be rejected.
	rules := []piiCustomRule{
		{Name: "greedy", Pattern: `.*`, Enabled: true},
	}
	got := parseCustomRules(logger, rules)
	if len(got) != 0 {
		t.Errorf("expected pattern matching empty string to be skipped, got %d patterns", len(got))
	}
}

func TestParseCustomRules_MixedValidity(t *testing.T) {
	logger := zaptest.NewLogger(t)
	rules := []piiCustomRule{
		{Name: "good", Pattern: `\bACCT-\d{4}\b`, Enabled: true},
		{Name: "bad_regex", Pattern: `[broken`, Enabled: true},
		{Name: "disabled", Pattern: `\bsecret\b`, Enabled: false},
	}
	got := parseCustomRules(logger, rules)
	if len(got) != 1 {
		t.Fatalf("expected 1 valid pattern, got %d", len(got))
	}
	if got[0].name != "good" {
		t.Errorf("expected name=good, got %q", got[0].name)
	}
}

// Ensure the zap import is exercised (it is used by zaptest indirectly; this
// line satisfies the compiler if the direct reference is removed).
var _ = (*zap.Logger)(nil)
