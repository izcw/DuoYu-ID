package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicImagesStayInsideTheirDirectory(t *testing.T) {
	s := &Server{data: t.TempDir()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /avatars/{id}", s.serveAvatar)
	mux.HandleFunc("GET /project-logos/{id}", s.serveProjectLogo)
	external := t.TempDir()
	name := strings.Repeat("a", 43) + ".png"
	outside := filepath.Join(external, name)
	if err := os.WriteFile(outside, []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{"avatars", "project-logos"} {
		t.Run(folder, func(t *testing.T) {
			dir := filepath.Join(s.data, folder)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, name)
			check := func(want int) {
				t.Helper()
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest("GET", "/"+folder+"/"+name, nil))
				if rec.Code != want {
					t.Errorf("served public image: got %d want %d", rec.Code, want)
				}
				if folder == "project-logos" && s.validProjectLogo("/project-logos/"+name) != (want == 200) {
					t.Error("logo validation disagrees with file boundary")
				}
			}
			if err := os.WriteFile(file, []byte("valid-path"), 0600); err != nil {
				t.Fatal(err)
			}
			check(200)
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, file); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			check(404)
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(dir); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, dir); err != nil {
				t.Fatal(err)
			}
			check(404)
		})
	}
}
