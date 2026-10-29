package main

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func (s *Server) avatar(w http.ResponseWriter, r *http.Request) {
	u, yes := s.require(w, r, false)
	if !yes {
		return
	}
	if r.ParseMultipartForm(2<<20) != nil {
		fail(w, 400, "图片不能超过 2 MB")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, h, e := r.FormFile("avatar")
	if e != nil || h.Size > 2<<20 {
		fail(w, 400, "请选择不超过 2 MB 的图片")
		return
	}
	defer f.Close()
	var b bytes.Buffer
	if _, e = b.ReadFrom(f); e != nil || b.Len() > 2<<20 {
		fail(w, 400, "图片过大")
		return
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(b.Bytes()))
	if e != nil || (format != "jpeg" && format != "png" && format != "webp") || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 20000000 {
		fail(w, 400, "只支持 JPEG、PNG、WebP，最多 2000 万像素")
		return
	}
	im, _, e := image.Decode(bytes.NewReader(b.Bytes()))
	if e != nil {
		fail(w, 400, "无法读取图片")
		return
	}
	bounds := im.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	x := bounds.Min.X + (bounds.Dx()-side)/2
	y := bounds.Min.Y + (bounds.Dy()-side)/2
	out := image.NewRGBA(image.Rect(0, 0, 256, 256))
	draw.CatmullRom.Scale(out, out.Bounds(), im, image.Rect(x, y, x+side, y+side), draw.Src, nil)
	id := randomID() + ".png"
	p := filepath.Join(s.data, "avatars", id)
	file, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		fail(w, 500, "保存失败")
		return
	}
	e = png.Encode(file, out)
	file.Close()
	if e != nil {
		os.Remove(p)
		fail(w, 500, "保存失败")
		return
	}
	avatar := "/avatars/" + id
	if _, e = s.db.Exec(r.Context(), "UPDATE users SET avatar=$1 WHERE id=$2", avatar, u.ID); e != nil {
		os.Remove(p)
		fail(w, 500, "保存失败")
		return
	}
	ok(w, map[string]string{"avatar": avatar})
}
func (s *Server) serveAvatar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if len(id) != 47 || !strings.HasSuffix(id, ".png") || strings.ContainsAny(id, "/\\.") && !strings.HasSuffix(id, ".png") {
		http.NotFound(w, r)
		return
	}
	for _, c := range strings.TrimSuffix(id, ".png") {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
	http.ServeFile(w, r, filepath.Join(s.data, "avatars", id))
}
