package cg

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (s *Server) frontendRoutes(r chi.Router) {
	if s.config.WebRoot != "" {
		r.Mount("/", &frontendHandler{
			webRoot: s.config.WebRoot,
		})
	}
}

type frontendHandler struct {
	webRoot string
}

func (f *frontendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	httpFS := http.Dir(f.webRoot)

	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}
	upath = path.Clean(upath)

	if path.Ext(upath) == "" {
		upath += ".html"
	}

	file, err := httpFS.Open(upath)
	if err != nil {
		http.ServeFile(w, r, filepath.Join(f.webRoot, "index.html"))
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.ServeFile(w, r, filepath.Join(f.webRoot, "index.html"))
		return
	}

	http.ServeContent(w, r, upath, info.ModTime(), file)
}
