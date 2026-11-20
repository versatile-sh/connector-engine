package console

import (
	"io/fs"
	"net/http"
	"strings"
)

func (c *Console) spa(w http.ResponseWriter, r *http.Request) {
	if c.Web == nil {
		http.Error(w, "the console frontend is not built", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(name, "api/") {
		http.NotFound(w, r)
		return
	}
	if name != "" && !strings.HasSuffix(name, "/") {
		if f, err := c.Web.Open(name); err == nil {
			st, err := f.Stat()
			f.Close()
			if err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, c.Web, name)
				return
			}
		}
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
	}
	index, err := fs.ReadFile(c.Web, "index.html")
	if err != nil {
		http.Error(w, "the console frontend is not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(index)
}
