package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestLoginAndScanConsentLabel(t *testing.T) {
	label := regexp.MustCompile(`<label class="legal-consent">(.*?)</label>`)
	input := regexp.MustCompile(`<input\b[^>]*id="legal-accept"[^>]*>`)
	checked := regexp.MustCompile(`\bchecked(?:\s|=|>)`)
	for _, name := range []string{"templates/login.html", "templates/scan.html"} {
		t.Run(name, func(t *testing.T) {
			body, err := assets.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			match := label.FindStringSubmatch(string(body))
			if len(match) != 2 || strings.Contains(match[1], "18周岁") || !strings.Contains(match[1], "我已阅读并同意") ||
				!strings.Contains(match[1], `href="/legal/terms"`) || !strings.Contains(match[1], `href="/legal/privacy"`) {
				t.Fatal("Expected consent text and links to both legal documents")
			}
			checkbox := input.FindString(match[1])
			if checkbox == "" || checked.MatchString(checkbox) {
				t.Fatal("Consent must remain an unchecked, explicit user action")
			}
		})
	}
}
