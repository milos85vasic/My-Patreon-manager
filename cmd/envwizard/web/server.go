package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/api"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
)

//go:embed static
var staticFiles embed.FS

type Server struct {
	apiServer *api.Server
	mux       *http.ServeMux
}

func NewServer(w *core.Wizard) *Server {
	s := &Server{
		apiServer: api.NewServer(w),
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.Handle("/api/", s.apiServer)

	staticFS, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))
	s.mux.Handle("/", fileServer)
}
