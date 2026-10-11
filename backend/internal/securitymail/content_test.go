package securitymail

import (
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestAlternativeMailPreservesPlainAndHTMLLinks(t *testing.T) {
	link := "https://app.example.com/register/verify#id=abc&token=def"
	kind, body, err := mailContent("Verify: "+link, `<a href="https://app.example.com/register/verify#id=abc&amp;token=def">Verify my email</a>`)
	if err != nil {
		t.Fatal(err)
	}
	media, params, err := mime.ParseMediaType(kind)
	if err != nil || media != "multipart/alternative" {
		t.Fatal("invalid MIME")
	}
	reader := multipart.NewReader(strings.NewReader(body), params["boundary"])
	for _, expected := range []string{link, "Verify my email"} {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(part)
		if err != nil || !strings.Contains(string(content), expected) {
			t.Fatalf("lost content: %s %v", content, err)
		}
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatal("unexpected extra part")
	}
}

func TestPlainSecurityMailStillPlain(t *testing.T) {
	kind, body, err := mailContent("challenge-code", "")
	if err != nil || kind != "text/plain; charset=UTF-8" || body != "challenge-code" {
		t.Fatal("plain challenge delivery changed")
	}
}
