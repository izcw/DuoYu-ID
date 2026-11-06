package main

import (
	"bytes"
	"golang.org/x/image/draw"
	"image"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var projectLogoPath = regexp.MustCompile(`^/project-logos/[A-Za-z0-9_-]{43}\.png$`)

func (s *Server) validProjectLogo(value string) bool {
	if !projectLogoPath.MatchString(value) {
		return false
	}
	file, _, err := s.openPublicImage("project-logos", strings.TrimPrefix(value, "/project-logos/"))
	if err != nil {
		return false
	}
	file.Close()
	return true
}
func (s *Server) uploadProjectLogo(w http.ResponseWriter, r *http.Request) {
	if _, yes := s.require(w, r, true); !yes {
		return
	}
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		fail(w, 400, "请选择不超过 2 MB 的图片")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, h, err := r.FormFile("logo")
	if err != nil {
		fail(w, 400, "请选择项目 Logo")
		return
	}
	defer f.Close()
	if h.Size > 2<<20 {
		fail(w, 400, "Logo 不能超过 2 MB")
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		fail(w, 400, "Logo 不能超过 2 MB")
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 20000000 {
		fail(w, 400, "支持 PNG、JPEG、WebP，最多 2000 万像素")
		return
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		fail(w, 400, "无法读取图片")
		return
	}
	width, height := cfg.Width, cfg.Height
	if max(width, height) > 512 {
		width = max(1, width*512/max(cfg.Width, cfg.Height))
		height = max(1, height*512/max(cfg.Width, cfg.Height))
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(out, out.Bounds(), im, im.Bounds(), draw.Src, nil)
	dir, err := publicImageDir(s.data, "project-logos", true)
	if err != nil {
		fail(w, 500, "Logo 保存失败")
		return
	}
	defer dir.Close()
	id := randomID() + ".png"
	if err = writePublicImage(dir, id, out); err != nil {
		fail(w, 500, "Logo 保存失败")
		return
	}
	ok(w, map[string]string{"logo": "/project-logos/" + id})
}
func (s *Server) serveProjectLogo(w http.ResponseWriter, r *http.Request) {
	s.servePublicImage(w, r, "project-logos")
}
