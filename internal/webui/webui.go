// Package webui embeds the same dashboard used by the public browser demo.
package webui

import (
	"embed"
	"html/template"
	"io"
	"net/http"
)

//go:embed index.html dashboard.css dashboard.js favicon.svg
var files embed.FS

var page = template.Must(template.ParseFS(files, "index.html"))

type Options struct {
	Mode  string `json:"mode"`
	Token string `json:"token,omitempty"`
}

func Write(w io.Writer, options Options) error {
	return page.Execute(w, options)
}

func Asset(w http.ResponseWriter, r *http.Request) bool {
	name := r.URL.Path
	if name != "/dashboard.css" && name != "/dashboard.js" && name != "/favicon.svg" {
		return false
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	if name == "/dashboard.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else if name == "/dashboard.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	b, _ := files.ReadFile(name[1:])
	w.Write(b)
	return true
}
