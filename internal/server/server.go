package server

import (
	"fmt"
	"log/slog"
	"net/http"
)

type Server struct {
	mux  *http.ServeMux
	port string
}

func NewHTTPServer(port string) *Server {
	return &Server{
		mux:  http.NewServeMux(),
		port: port,
	}
}

func (s *Server) RegisterRoutes(pattern string, hander http.HandlerFunc) {
	s.mux.HandleFunc(pattern, hander)
}

func (s *Server) Start() error {
	slog.Info("Starting HTTP server", "port", s.port)
	return http.ListenAndServe(fmt.Sprintf(":%s", s.port), s.mux)
}
