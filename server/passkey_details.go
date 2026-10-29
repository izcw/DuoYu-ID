package main

import (
	"github.com/go-webauthn/webauthn/webauthn"
	"strings"
)

// This describes the registering browser, not the physical authenticator.
func registrationDevice(agent string) string {
	os, browser := "未知系统", "未知浏览器"
	for _, item := range [][2]string{{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"Windows", "Windows"}, {"Macintosh", "macOS"}, {"Linux", "Linux"}} {
		if strings.Contains(agent, item[0]) {
			os = item[1]
			break
		}
	}
	for _, item := range [][2]string{{"Edg/", "Edge"}, {"EdgiOS/", "Edge"}, {"EdgA/", "Edge"}, {"OPR/", "Opera"}, {"FxiOS/", "Firefox"}, {"Firefox/", "Firefox"}, {"CriOS/", "Chrome"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}} {
		if strings.Contains(agent, item[0]) {
			browser = item[1]
			break
		}
	}
	return os + " · " + browser
}

func passkeyDetails(c webauthn.Credential) (string, []string) {
	kind := "未知验证器"
	switch string(c.Authenticator.Attachment) {
	case "platform":
		kind = "设备内置验证器"
	case "cross-platform":
		kind = "外部验证器 / 跨设备通行密钥"
	}
	methods := []string{}
	labels := map[string]string{"internal": "设备内验证", "hybrid": "手机 / 跨设备验证", "usb": "USB 安全密钥", "nfc": "NFC 安全密钥", "ble": "蓝牙验证", "smart-card": "智能卡"}
	seen := map[string]bool{}
	for _, transport := range c.Transport {
		if label := labels[string(transport)]; label != "" && !seen[label] {
			methods = append(methods, label)
			seen[label] = true
		}
	}
	if len(methods) == 0 {
		methods = append(methods, "浏览器未提供")
	}
	return kind, methods
}
