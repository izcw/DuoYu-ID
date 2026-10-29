package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"testing"
)

func testUploadProjectLogo(t *testing.T, client *http.Client, origin string) string {
	t.Helper()
	upload := func(c *http.Client, content []byte, want int) string {
		t.Helper()
		var body bytes.Buffer
		m := multipart.NewWriter(&body)
		f, _ := m.CreateFormFile("logo", "test.png")
		f.Write(content)
		m.Close()
		r, _ := http.NewRequest("POST", origin+"/api/admin/project-logos", &body)
		r.Header.Set("Content-Type", m.FormDataContentType())
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Duoyu-Request", "1")
		r.Header.Set("X-Duoyu-Legal-Version", legalVersion)
		r.Header.Set("X-Duoyu-Adult", "true")
		resp, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != want {
			t.Fatalf("upload %d: %v", resp.StatusCode, out)
		}
		if want == 200 {
			return out["data"].(map[string]any)["logo"].(string)
		}
		return ""
	}
	var data bytes.Buffer
	png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 1024, 512)))
	upload(http.DefaultClient, data.Bytes(), 401)
	upload(client, []byte("<svg onload=alert(1)>"), 400)
	upload(client, make([]byte, (2<<20)+1), 400)
	logo := upload(client, data.Bytes(), 200)
	if !projectLogoPath.MatchString(logo) {
		t.Fatal("unexpected logo URL")
	}
	resp, err := http.Get(origin + logo)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	im, err := png.Decode(resp.Body)
	if err != nil || im.Bounds().Dx() != 512 || im.Bounds().Dy() != 256 {
		t.Fatal("logo resize/proportion failed", err)
	}
	_, _, _, alpha := im.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatal("lost transparency")
	}
	return logo
}
