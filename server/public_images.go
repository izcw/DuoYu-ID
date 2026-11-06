package main

import (
	"errors"
	"image"
	"image/png"
	"net/http"
	"os"
	"regexp"
)

var publicImageID = regexp.MustCompile(`^[A-Za-z0-9_-]{43}\.png$`)

// Keep both reads and writes anchored to an actual image directory. A backup
// may contain links, and neither a link nor a replacement inode is public data.
func publicImageDir(data, folder string, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(data)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir(folder, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	before, err := root.Lstat(folder)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() {
		return nil, os.ErrInvalid
	}
	dir, err := root.OpenRoot(folder)
	if err != nil {
		return nil, err
	}
	actual, err := dir.Stat(".")
	if err != nil || !os.SameFile(before, actual) {
		dir.Close()
		return nil, os.ErrInvalid
	}
	return dir, nil
}

func (s *Server) openPublicImage(folder, id string) (*os.File, os.FileInfo, error) {
	if !publicImageID.MatchString(id) {
		return nil, nil, os.ErrInvalid
	}
	dir, err := publicImageDir(s.data, folder, false)
	if err != nil {
		return nil, nil, err
	}
	defer dir.Close()
	before, err := dir.Lstat(id)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, os.ErrInvalid
	}
	file, err := dir.Open(id)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !os.SameFile(before, info) {
		file.Close()
		return nil, nil, os.ErrInvalid
	}
	return file, info, nil
}

func (s *Server) servePublicImage(w http.ResponseWriter, r *http.Request, folder string) {
	id := r.PathValue("id")
	f, info, err := s.openPublicImage(folder, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
	http.ServeContent(w, r, id, info.ModTime(), f)
}

func writePublicImage(dir *os.Root, id string, im image.Image) error {
	f, err := dir.OpenFile(id, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = png.Encode(f, im)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = dir.Remove(id)
	}
	return err
}
