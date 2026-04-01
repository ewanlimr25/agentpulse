package piimaskerproc

import (
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// piiPattern pairs a human-readable name with a compiled RE2 regex.
// Go's regexp package uses RE2 semantics (linear-time execution, no
// backtracking catastrophes). Custom patterns loaded from Postgres are
// therefore safe from ReDoS by design.
type piiPattern struct {
	name string
	re   *regexp.Regexp
}

// piiCustomRule is a local mirror of the backend domain type, avoiding a
// cross-module import. JSON tags match the Postgres jsonb column layout.
type piiCustomRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Enabled bool   `json:"enabled"`
}

// builtinPatterns returns the 14 compiled built-in PII/secret patterns.
// All patterns are compiled once at package init and reused for every span.
func builtinPatterns() []piiPattern {
	return []piiPattern{
		{
			name: "credit_card",
			re:   regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`),
		},
		{
			name: "ssn",
			re:   regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		},
		{
			name: "email",
			re:   regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
		},
		{
			name: "jwt",
			re:   regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
		},
		{
			name: "api_key_openai",
			re:   regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`),
		},
		{
			name: "api_key_anthropic",
			re:   regexp.MustCompile(`\bsk-ant-[A-Za-z0-9\-_]{32,}\b`),
		},
		{
			name: "bearer_inline",
			re:   regexp.MustCompile(`\bBearer\s+[A-Za-z0-9._~+/\-]+=*\b`),
		},
		{
			name: "aws_access_key",
			re:   regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		},
		{
			name: "github_pat",
			re:   regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b`),
		},
		{
			name: "stripe_live",
			re:   regexp.MustCompile(`\bsk_live_[A-Za-z0-9]{24,}\b`),
		},
		{
			name: "google_api_key",
			re:   regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`),
		},
		{
			name: "pem_header",
			re:   regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
		},
		{
			name: "slack_token",
			re:   regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9\-]{10,}\b`),
		},
		{
			name: "phone_us",
			re:   regexp.MustCompile(`\b(?:\+1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`),
		},
	}
}

// buildCombinedRegex creates a single alternation regex from a slice of
// patterns. This is used as a fast pre-filter: if no match, skip individual
// pattern scanning entirely.
func buildCombinedRegex(patterns []piiPattern) *regexp.Regexp {
	if len(patterns) == 0 {
		// Match nothing.
		return regexp.MustCompile(`(?:^$\bNEVERMATCH\b)`)
	}
	parts := make([]string, len(patterns))
	for i, p := range patterns {
		parts[i] = p.re.String()
	}
	combined := "(?:" + strings.Join(parts, "|") + ")"
	return regexp.MustCompile(combined)
}

// redact replaces all PII matches in value with [REDACTED:<name>] tokens.
// It first runs the combined alternation regex as a fast pre-filter; if
// there are no matches at all the original string is returned with count=0.
// When the pre-filter fires it runs individual patterns to produce named
// replacement tokens.
func redact(value string, patterns []piiPattern, combined *regexp.Regexp) (redacted string, count int) {
	if value == "" {
		return value, 0
	}
	// Fast pre-filter using the combined alternation regex.
	if !combined.MatchString(value) {
		return value, 0
	}
	// At least one match — run each pattern individually for named tokens.
	result := value
	for _, p := range patterns {
		replacement := fmt.Sprintf("[REDACTED:%s]", p.name)
		matches := p.re.FindAllStringIndex(result, -1)
		if len(matches) == 0 {
			continue
		}
		result = p.re.ReplaceAllString(result, replacement)
		count += len(matches)
	}
	return result, count
}

// parseCustomRules compiles enabled custom rules from Postgres into piiPatterns.
// Invalid regex or patterns that match the empty string are logged and skipped
// rather than panicking, so a bad rule never takes down the processor.
func parseCustomRules(logger *zap.Logger, rules []piiCustomRule) []piiPattern {
	var out []piiPattern
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			logger.Warn("piimaskerproc: skipping custom rule with invalid regex",
				zap.String("name", r.Name),
				zap.String("pattern", r.Pattern),
				zap.Error(err),
			)
			continue
		}
		// Reject patterns that match the empty string — they would corrupt every
		// attribute value by prepending a REDACTED token.
		if re.MatchString("") {
			logger.Warn("piimaskerproc: skipping custom rule whose pattern matches empty string",
				zap.String("name", r.Name),
				zap.String("pattern", r.Pattern),
			)
			continue
		}
		out = append(out, piiPattern{name: r.Name, re: re})
	}
	return out
}
