package web

import "net/http"

func (s *Server) handleAPIDiagnostics(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.diagnostics.Report(r.Context()))
}
