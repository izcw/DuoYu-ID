package main

import (
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"testing"
)

func TestConfidentialWebLoopback(t *testing.T) {
	s := &Server{}
	const uri = "http://127.0.0.1:4323/api/auth/callback"
	if !s.validAppRedirect(uri, "web", false) || s.validAppRedirect(uri, "web", true) || s.validAppRedirect(uri, "spa", true) || s.validRedirect(uri) {
		t.Fatal("incorrect callback registration policy")
	}
	for _, invalid := range []string{"http://localhost:4323/cb", "http://192.168.1.1:4323/cb", "http://127.0.0.1/cb", "http://127.0.0.1:65536/cb", "http://127.0.0.1:*/cb", uri + "?a=b", uri + "#fragment"} {
		if s.validAppRedirect(invalid, "web", false) {
			t.Errorf("accepted %q", invalid)
		}
	}
	app := &App{Platform: "web", Redirects: []string{uri}}
	if err := op.ValidateAuthReqRedirectURI(app, uri, oidc.ResponseTypeCode); err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{"http://127.0.0.1:4324/api/auth/callback", uri + "/", "http://localhost:4323/api/auth/callback"} {
		if op.ValidateAuthReqRedirectURI(app, other, oidc.ResponseTypeCode) == nil {
			t.Errorf("matched unregistered URI %q", other)
		}
	}
}
