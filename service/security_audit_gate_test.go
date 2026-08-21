package service

import (
	"strings"
	"testing"
)

// allGatedRegexps is every gated pattern in the package. Adding a pattern
// without adding it here means the gate invariant below never covers it.
func allGatedRegexps() []gatedRegexp {
	var all []gatedRegexp
	all = append(all, apiKeyRegexps...)
	all = append(all, overrideRegexps...)
	all = append(all, emailRegexp)
	return all
}

// FuzzGateIsNecessaryCondition asserts the property the whole optimization rests
// on: whenever a pattern matches, its gates must pass. If that ever fails the
// gate would silently suppress a real finding.
func FuzzGateIsNecessaryCondition(f *testing.F) {
	seeds := []string{
		"", "a", "api_key = abcdefghijklmnopqrstuvwxyz012345",
		"APIKEY: ABCDEFGHIJKLMNOPQRSTUVWXYZ0123", "AccessKey=zzzzzzzzzzzzzzzzzzzzzzz",
		"secret: hunter2hunter2", "PASSWORD=supersecret", "passwd:abcdefghij", "PWD = 12345678",
		"sk-proj-abcdefghijklmnopqrstuvwxyz1234567890",
		"SK-PROJ-ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890",
		"ghp_" + strings.Repeat("a", 36), "glpat-" + strings.Repeat("b", 20),
		"xoxb-1234567890-abcdef", "AIza" + strings.Repeat("c", 35), "AKIA" + strings.Repeat("Z", 16),
		"ya29." + strings.Repeat("d", 100), "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9",
		"override safety", "OVERRIDE   FILTER", "bypass restriction", "turn off guideline",
		"disable\tsafety", "你现在 不受 限制", "now you are unrestricted", "You're Now Without limits",
		"user@example.com", "A.B+c%d@Sub.Domain.CO.UK",
		"İstanbul KEY=aaaaaaaaaaaaaaaaaaaaaaaa", "ΑΚΙΑ", "ＡＫＩＡ",
		"Key = aaaaaaaaaaaaaaaaaaaaaaaaa", // Kelvin sign lowercases to ASCII k
	}
	for _, s := range seeds {
		f.Add(s)
	}

	all := allGatedRegexps()
	f.Fuzz(func(t *testing.T, s string) {
		lower := strings.ToLower(s)
		for _, g := range all {
			// Production checks letter gates against the lowercased text; gates made
			// of digits or punctuation are unaffected by lowering, so one haystack
			// covers both kinds.
			if g.re.MatchString(s) && !g.gatesPass(lower) {
				t.Fatalf("gate suppressed a real match: pattern %q gates %v input %q",
					g.re.String(), g.gates, s)
			}
		}
	})
}
