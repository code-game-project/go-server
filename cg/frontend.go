package cg

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (s *Server) frontendRoutes(r chi.Router) {
	if s.config.Frontend != nil {
		r.Mount("/", &frontendHandler{
			frontend: s.config.Frontend,
		})
	}
}

type frontendHandler struct {
	frontend fs.FS
}

func (f *frontendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	httpFS := http.FS(f.frontend)

	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}
	upath = path.Clean(upath)

	var file http.File
	var err error
	file, err = httpFS.Open(upath)
	if err != nil {
		file, err = httpFS.Open(upath + ".html")
		if err != nil {
			file, err = httpFS.Open("index.html")
			if err != nil {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}
		}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if info.IsDir() {
		file, err = httpFS.Open(path.Join(strings.TrimPrefix(upath, "/"), "index.html"))
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		defer file.Close()
	}

	http.ServeContent(w, r, upath, info.ModTime(), file)
}
