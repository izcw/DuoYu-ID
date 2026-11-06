package main

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"net/http"

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
	if e != nil {
		fail(w, 400, "请选择不超过 2 MB 的图片")
		return
	}
	defer f.Close()
	if h.Size > 2<<20 {
		fail(w, 400, "请选择不超过 2 MB 的图片")
		return
	}
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
	dir, e := publicImageDir(s.data, "avatars", true)
	if e != nil {
		fail(w, 500, "保存失败")
		return
	}
	defer dir.Close()
	if e = writePublicImage(dir, id, out); e != nil {
		fail(w, 500, "保存失败")
		return
	}
	avatar := "/avatars/" + id
	tag, e := s.db.Exec(r.Context(), "UPDATE users SET avatar=$1 WHERE id=$2 AND enabled", avatar, u.ID)
	if e != nil || tag.RowsAffected() != 1 {
		dir.Remove(id)
		fail(w, 500, "保存失败")
		return
	}
	ok(w, map[string]string{"avatar": avatar})
}
func (s *Server) serveAvatar(w http.ResponseWriter, r *http.Request) {
	s.servePublicImage(w, r, "avatars")
}
