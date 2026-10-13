package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aegion-dynamic/apismith/openapi"
	"aegion-dynamic/apismith/request"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSavedList(w http.ResponseWriter, r *http.Request) {
	items, err := s.lib.ListSaved()
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleSavedPut(w http.ResponseWriter, r *http.Request) {
	var in request.SavedRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.lib.PutSaved(in); err != nil {
		writeLibErr(w, err)
		return
	}
	saved, err := s.lib.GetSaved(strings.TrimSpace(in.Name))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleSavedDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.lib.DeleteSaved(chi.URLParam(r, "name")); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSequenceList(w http.ResponseWriter, r *http.Request) {
	items, err := s.lib.ListSequences()
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleSequencePut(w http.ResponseWriter, r *http.Request) {
	var in request.Sequence
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.lib.PutSequence(in); err != nil {
		writeLibErr(w, err)
		return
	}
	saved, err := s.lib.GetSequence(strings.TrimSpace(in.Name))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleSequenceGet(w http.ResponseWriter, r *http.Request) {
	seq, err := s.lib.GetSequence(chi.URLParam(r, "name"))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, seq)
}

func (s *Server) handleSequenceDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.lib.DeleteSequence(chi.URLParam(r, "name")); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type runBody struct {
	Name              string           `json:"name"`
	Sequence          request.Sequence `json:"sequence"`
	Environment       string           `json:"environment"`
	BaseURL           string           `json:"base_url"`
	AuthMode          request.AuthMode `json:"auth_mode"`
	JWT               string           `json:"jwt"`
	CustomAuth        string           `json:"custom_authorization"`
	ConfirmProduction bool             `json:"confirm_production"`
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var in runBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	seq := in.Sequence
	if len(seq.Steps) == 0 && strings.TrimSpace(in.Name) != "" {
		loaded, err := s.lib.GetSequence(strings.TrimSpace(in.Name))
		if err != nil {
			writeLibErr(w, err)
			return
		}
		seq = loaded
	}
	if strings.TrimSpace(seq.Name) == "" {
		seq.Name = strings.TrimSpace(in.Name)
	}
	if len(seq.Steps) == 0 {
		writeError(w, http.StatusBadRequest, "a sequence needs at least one step")
		return
	}
	if in.Environment == "" {
		in.Environment = s.cfg.DefaultEnvironment
	}
	env := s.cfg.Find(in.Environment)
	if env == nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown environment %q", in.Environment))
		return
	}
	if in.AuthMode == "" {
		in.AuthMode = request.AuthJWT
	}
	if in.AuthMode == request.AuthJWT && in.JWT == "" {
		if tok, ok := s.auth.AccessToken(); ok {
			in.JWT = tok
		}
	}
	baseURL, err := request.ResolveBaseURL(env.BaseURL, in.BaseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.runMu.Lock()
	result := request.RunSequence(s.exec, s.lib, seq, request.RunOptions{
		BaseURL:    baseURL,
		Production: env.Production,
		Confirm:    in.ConfirmProduction,
		AuthMode:   in.AuthMode,
		JWT:        in.JWT,
		CustomAuth: in.CustomAuth,
		Vars:       s.vars.Snapshot(),
	})
	s.vars.Replace(result.Variables)
	s.runMu.Unlock()

	for i, step := range result.Steps {
		if step.Skipped || i >= len(seq.Steps) {
			continue
		}
		resolved, err := s.lib.ResolveStep(seq.Steps[i])
		if err != nil {
			resolved = seq.Steps[i]
		}
		mode := resolved.AuthMode
		if mode == "" {
			mode = in.AuthMode
		}
		s.history.Add(request.HistoryItem{
			Environment: in.Environment,
			Method:      resolved.Method,
			Path:        resolved.Path,
			PathParams:  resolved.PathParams,
			Query:       resolved.Query,
			Headers:     resolved.Headers,
			Body:        resolved.Body,
			BodyType:    resolved.BodyType,
			ContentType: resolved.ContentType,
			AuthMode:    mode,
			Status:      step.Status,
			DurationMS:  step.DurationMS,
			OK:          step.OK,
			Error:       step.Error,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleVarsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.vars.Snapshot())
}

func (s *Server) handleVarsPut(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	clean := map[string]string{}
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		clean[k] = v
	}
	s.vars.Replace(clean)
	writeJSON(w, http.StatusOK, s.vars.Snapshot())
}

func (s *Server) handleVarsDelete(w http.ResponseWriter, r *http.Request) {
	s.vars.Delete(chi.URLParam(r, "name"))
	writeJSON(w, http.StatusOK, s.vars.Snapshot())
}

func (s *Server) handleSnippet(w http.ResponseWriter, r *http.Request) {
	var in request.ExecuteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Environment == "" && s.cfg != nil {
		in.Environment = s.cfg.DefaultEnvironment
	}
	baseCfg := ""
	if s.cfg != nil {
		if env := s.cfg.Find(in.Environment); env != nil {
			baseCfg = env.BaseURL
		}
	}
	expanded := request.ExpandInput(in, s.vars.Snapshot(), 1)
	baseURL, err := request.ResolveBaseURL(baseCfg, expanded.BaseURL)
	if err != nil {
		baseURL = expanded.BaseURL
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"curl": request.Curl(baseURL, expanded),
		"call": request.CallCommand(expanded),
	})
}

func writeLibErr(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	status := http.StatusBadRequest
	if strings.Contains(err.Error(), "not found") {
		status = http.StatusNotFound
	}
	writeError(w, status, err.Error())
}

func applySchema(cat *openapi.Catalog, method, path string, out *request.ExecuteOutput) {
	if cat == nil || out == nil || out.Status == 0 || !looksJSON(out.ContentType, out.Body) {
		return
	}
	schema := cat.ResponseSchema(method, path, out.Status)
	if schema == nil {
		return
	}
	ok, notes := openapi.MatchBody(out.Body, schema)
	out.SchemaOK = &ok
	if len(notes) > 0 {
		out.SchemaNotes = notes
	}
}

func looksJSON(contentType, body string) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	trimmed := strings.TrimSpace(body)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}
