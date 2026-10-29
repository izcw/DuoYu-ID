package main

import (
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"reflect"
	"testing"
)

func TestPasskeyDetails(t *testing.T) {
	for _, tc := range []struct {
		attachment string
		transports []protocol.AuthenticatorTransport
		kind       string
		methods    []string
	}{
		{"platform", []protocol.AuthenticatorTransport{"internal", "hybrid"}, "设备内置验证器", []string{"设备内验证", "手机 / 跨设备验证"}},
		{"cross-platform", []protocol.AuthenticatorTransport{"usb", "nfc", "usb"}, "外部验证器 / 跨设备通行密钥", []string{"USB 安全密钥", "NFC 安全密钥"}},
		{"", nil, "未知验证器", []string{"浏览器未提供"}},
	} {
		c := webauthn.Credential{Authenticator: webauthn.Authenticator{Attachment: protocol.AuthenticatorAttachment(tc.attachment)}, Transport: tc.transports}
		kind, methods := passkeyDetails(c)
		if kind != tc.kind || !reflect.DeepEqual(methods, tc.methods) {
			t.Fatal(kind, methods)
		}
	}
	for _, tc := range [][2]string{{"Mozilla/5.0 (Windows NT 10.0) Chrome/120 Safari/537.36 Edg/120", "Windows · Edge"}, {"Mozilla/5.0 (iPhone) Version/17 Safari/605", "iPhone · Safari"}, {"Mozilla/5.0 (iPad) CriOS/120 Safari/605", "iPad · Chrome"}, {"Mozilla/5.0 (Linux; Android 14) Chrome/120 Safari/537", "Android · Chrome"}, {"", "未知系统 · 未知浏览器"}} {
		if got := registrationDevice(tc[0]); got != tc[1] {
			t.Fatal(got, tc[1])
		}
	}
}
