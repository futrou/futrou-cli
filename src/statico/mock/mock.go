// Package mock is a stateful stand-in for the Futrou Cloud statico and
// storage-files endpoints, for tests. Every request must carry the bearer token
// the server was created with, like Cloud, which authorises the caller's token
// and forwards the files to the node itself.
package mock

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Token is the bearer token the fake accepts.
const Token = "test-token"

// Version is a version of a website.
type StaticoVersion struct {
	Name   string
	Status string
	Error  string
	Number int
	Extra  map[string]interface{}
}

// Statico is a website with its versions and files (storage folder per version).
type Statico struct {
	ID, Name, ProjectID, WorkspaceID, RegionID string
	Public                                     bool
	Active                                     string
	Versions                                   []*StaticoVersion
	Files                                      map[string]map[string][]byte // version -> path -> content
}

// Server is the fake.
type Server struct {
	mu       sync.Mutex
	Staticos []*Statico
	// Workspace is the workspace of a website created without one (WorkspaceID when empty).
	Workspace string
	// Log holds "METHOD path" of every request in order.
	Log []string
	// Archives counts archive uploads and ArchiveFiles the files in them; RecursiveDeletes counts folders deleted in one request.
	Archives, ArchiveFiles, RecursiveDeletes int
	// MissingVersionStatus is the status for a version that does not exist (404 when 0): the real API answers 400 "Version not found".
	MissingVersionStatus int
	// RejectArchives answers an archive upload like an API whose Content-Type check does not exempt that route.
	RejectArchives bool
	// HangUploads, when set, makes a file or archive upload wait until its request is cancelled; it gets a value as each starts.
	HangUploads chan struct{}
	// FailTimes makes a request ("METHOD path") fail with a 500 that many times, then work.
	FailTimes map[string]int
	// FailWrites answers every file or archive upload with this status (0: none).
	FailWrites int
	// OmitDomain leaves publicDomain out of a website, like an API server that does not send it.
	OmitDomain bool
	// Created holds the body of every website creation.
	Created []map[string]interface{}
	// Unauthorized counts requests that did not carry the token.
	Unauthorized int
	// Fail makes "METHOD /path" answer with the status.
	Fail map[string]int
	next int
}

// WorkspaceID is the workspace of the websites AddStatico adds, a UUID like the real ones.
const WorkspaceID = "00000000-0000-4000-8000-0000000000aa"

// New returns an empty fake.
func New() *Server { return &Server{Fail: map[string]int{}, FailTimes: map[string]int{}} }

// AddStatico adds a website that has an active version with the files.
func (s *Server) AddStatico(name string, files map[string]string) *Statico {
	s.mu.Lock()
	defer s.mu.Unlock()
	statico := s.newSite(name, "proj-1", WorkspaceID)
	if files != nil {
		v := s.newVersion(statico, "")
		statico.Files[v.Name] = map[string][]byte{}
		for p, c := range files {
			statico.Files[v.Name][p] = []byte(c)
		}
		v.Status = "completed"
		statico.Active = v.Name
	}
	return statico
}

func (s *Server) newSite(name, project, workspace string) *Statico {
	s.next++
	statico := &Statico{ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", s.next), Name: name, ProjectID: project, WorkspaceID: workspace, Public: true, Files: map[string]map[string][]byte{}}
	s.Staticos = append(s.Staticos, statico)
	return statico
}

func (s *Server) newVersion(statico *Statico, name string) *StaticoVersion {
	if name == "" {
		name = fmt.Sprintf("v%d", len(statico.Versions)+1)
	}
	v := &StaticoVersion{Name: name, Status: "draft", Number: len(statico.Versions) + 1, Extra: map[string]interface{}{}}
	statico.Versions = append(statico.Versions, v)
	return v
}

// Statico returns the website named name, or nil.
func (s *Server) Statico(name string) *Statico {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, statico := range s.Staticos {
		if statico.Name == name {
			return statico
		}
	}
	return nil
}

// ActiveFiles returns the files of the served version as path -> content.
func (s *Server) ActiveFiles(name string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for _, statico := range s.Staticos {
		if statico.Name == name {
			for p, c := range statico.Files[statico.Active] {
				out[p] = string(c)
			}
		}
	}
	return out
}

// Count returns how many requests matched "METHOD path-prefix".
func (s *Server) Count(method, prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.Log {
		if strings.HasPrefix(e, method+" "+prefix) {
			n++
		}
	}
	return n
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func (s *Server) siteBody(statico *Statico) map[string]interface{} {
	body := map[string]interface{}{"id": statico.ID, "name": statico.Name, "projectId": statico.ProjectID, "workspaceId": statico.WorkspaceID, "regionId": statico.RegionID, "public": statico.Public, "storage": "statico-" + statico.ID, "activeVersion": statico.Active, "publicDomain": PublicDomain(statico)}
	if s.OmitDomain {
		delete(body, "publicDomain")
	}
	return body
}

func (s *Server) versionBody(statico *Statico, v *StaticoVersion) map[string]interface{} {
	body := map[string]interface{}{"name": v.Name, "number": v.Number, "status": v.Status, "storage": "statico-" + statico.ID, "files": len(statico.Files[v.Name])}
	if v.Error != "" {
		body["error"] = v.Error
	}
	for k, val := range v.Extra {
		body[k] = val
	}
	return body
}

// PublicDomain is the domain Cloud gives a website: statico-<last part of the id>.futrou.cloud.
func PublicDomain(statico *Statico) string {
	return "statico-" + statico.ID[strings.LastIndex(statico.ID, "-")+1:] + ".futrou.cloud"
}

// Handle serves a request; it reports false for paths that are not statico or storage routes.
func (s *Server) Handle(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.EscapedPath()
	if !strings.HasPrefix(path, "/v2/staticos") && !strings.HasPrefix(path, "/v2/storages/statico-") {
		return false
	}
	if s.HangUploads != nil && strings.HasPrefix(path, "/v2/storages/") && (r.Method == "PUT" || r.Method == "POST") {
		select {
		case s.HangUploads <- struct{}{}:
		default:
		}
		_, _ = io.Copy(io.Discard, r.Body) // the server notices a closed connection only after the body is read
		<-r.Context().Done()               // not holding the lock: the rest of the API keeps answering
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Log = append(s.Log, r.Method+" "+path)
	if r.Header.Get("Authorization") != "Bearer "+Token {
		s.Unauthorized++
		reply(w, 401, map[string]string{"message": "unauthorized"})
		return true
	}
	if s.FailWrites != 0 && strings.HasPrefix(path, "/v2/storages/") && (r.Method == "PUT" || r.Method == "POST") {
		reply(w, s.FailWrites, map[string]string{"message": "forced failure"})
		return true
	}
	if n := s.FailTimes[r.Method+" "+path]; n > 0 {
		s.FailTimes[r.Method+" "+path] = n - 1
		reply(w, 500, map[string]string{"message": "temporary failure"})
		return true
	}
	if status, ok := s.Fail[r.Method+" "+path]; ok {
		reply(w, status, map[string]string{"message": "forced failure"})
		return true
	}
	if strings.HasPrefix(path, "/v2/storages/") {
		s.storage(w, r, path)
	} else {
		s.staticos(w, r, strings.Split(strings.Trim(strings.TrimPrefix(path, "/v2/staticos"), "/"), "/"))
	}
	return true
}

func reply(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func notFound(w http.ResponseWriter) { reply(w, 404, map[string]string{"message": "not found"}) }

func (s *Server) byID(id string) *Statico {
	for _, statico := range s.Staticos {
		if statico.ID == id {
			return statico
		}
	}
	return nil
}

func (s *Server) staticos(w http.ResponseWriter, r *http.Request, parts []string) {
	var body map[string]interface{}
	if r.Method == "POST" || r.Method == "PATCH" {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if len(parts) == 1 && parts[0] == "" {
		switch r.Method {
		case "GET":
			if r.URL.Query().Get("workspaceId") == "" && r.URL.Query().Get("projectId") == "" {
				reply(w, 400, map[string]string{"message": "No workspaceId provided"}) // as the real API
				return
			}
			list := []map[string]interface{}{}
			for _, statico := range s.Staticos {
				if p := r.URL.Query().Get("projectId"); p != "" && p != statico.ProjectID {
					continue
				}
				if wsp := r.URL.Query().Get("workspaceId"); wsp != "" && wsp != statico.WorkspaceID {
					continue
				}
				list = append(list, s.siteBody(statico))
			}
			reply(w, 200, list)
		case "POST":
			s.Created = append(s.Created, body)
			name, _ := body["name"].(string)
			if name == "" {
				reply(w, 400, map[string]string{"message": "name required"})
				return
			}
			project, _ := body["projectId"].(string)
			workspace, _ := body["workspaceId"].(string)
			if project == "" && workspace == "" {
				reply(w, 400, map[string]string{"message": "workspaceId or projectId is required"})
				return
			}
			if project == "" {
				project = "proj-1"
			}
			if workspace == "" {
				workspace = s.Workspace
				if workspace == "" {
					workspace = WorkspaceID
				}
			}
			statico := s.newSite(name, project, workspace)
			statico.RegionID, _ = body["regionId"].(string)
			if public, ok := body["public"].(bool); ok {
				statico.Public = public
			}
			reply(w, 201, s.siteBody(statico))
		default:
			reply(w, 405, map[string]string{"message": "method not allowed"})
		}
		return
	}
	statico := s.byID(parts[0])
	if statico == nil {
		notFound(w)
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			reply(w, 200, s.siteBody(statico))
		case "PATCH":
			if name, ok := body["name"].(string); ok {
				statico.Name = name
			}
			if public, ok := body["public"].(bool); ok {
				statico.Public = public
			}
			if region, ok := body["regionId"].(string); ok {
				statico.RegionID = region
			}
			reply(w, 200, s.siteBody(statico))
		case "DELETE":
			for i, x := range s.Staticos {
				if x == statico {
					s.Staticos = append(s.Staticos[:i:i], s.Staticos[i+1:]...)
					break
				}
			}
			w.WriteHeader(204)
		default:
			reply(w, 405, map[string]string{"message": "method not allowed"})
		}
		return
	}
	if parts[1] != "versions" {
		notFound(w)
		return
	}
	s.versions(w, r, statico, parts[2:], body)
}

func (s *Server) findVersion(statico *Statico, name string) *StaticoVersion {
	if name == "active" {
		name = statico.Active
	}
	for _, v := range statico.Versions {
		if v.Name == name && name != "" {
			return v
		}
	}
	return nil
}

func (s *Server) versions(w http.ResponseWriter, r *http.Request, statico *Statico, parts []string, body map[string]interface{}) {
	if len(parts) == 0 || parts[0] == "" {
		switch r.Method {
		case "GET":
			items := []map[string]interface{}{}
			for _, v := range statico.Versions {
				items = append(items, s.versionBody(statico, v))
			}
			reply(w, 200, map[string]interface{}{"items": items})
		case "POST":
			name, _ := body["name"].(string)
			// a draft or failed version is replaced
			for i, v := range statico.Versions {
				if v.Status != "completed" {
					delete(statico.Files, v.Name)
					statico.Versions = append(statico.Versions[:i:i], statico.Versions[i+1:]...)
					break
				}
			}
			v := s.newVersion(statico, name)
			files := map[string][]byte{}
			if from, _ := body["from"].(string); from != "empty" && statico.Active != "" {
				for p, c := range statico.Files[statico.Active] {
					files[p] = c
				}
			}
			statico.Files[v.Name] = files
			for _, k := range []string{"defaultFile", "defaultStatus", "fallbackFile", "fallbackStatus", "redirects"} {
				if val, ok := body[k]; ok {
					v.Extra[k] = val
				}
			}
			reply(w, 200, s.versionBody(statico, v))
		default:
			reply(w, 405, map[string]string{"message": "method not allowed"})
		}
		return
	}
	v := s.findVersion(statico, parts[0])
	if v == nil {
		if s.MissingVersionStatus != 0 {
			reply(w, s.MissingVersionStatus, map[string]string{"message": "Version not found"})
			return
		}
		notFound(w)
		return
	}
	if len(parts) == 2 && parts[1] == "activate" && r.Method == "POST" {
		if len(statico.Files[v.Name]) == 0 {
			v.Status, v.Error = "failed", "no files"
			reply(w, 413, map[string]string{"message": "version has no files"})
			return
		}
		v.Status = "completed"
		statico.Active = v.Name
		reply(w, 200, s.versionBody(statico, v))
		return
	}
	switch r.Method {
	case "GET":
		reply(w, 200, s.versionBody(statico, v))
	case "PATCH":
		if st, ok := body["status"].(string); ok {
			v.Status = st
		}
		if e, ok := body["error"].(string); ok {
			v.Error = e
		}
		for _, k := range []string{"defaultFile", "defaultStatus", "fallbackFile", "fallbackStatus", "redirects"} {
			if val, ok := body[k]; ok {
				v.Extra[k] = val
			}
		}
		reply(w, 200, s.versionBody(statico, v))
	case "DELETE":
		if statico.Active == v.Name {
			reply(w, 409, map[string]string{"message": "the active version cannot be deleted"})
			return
		}
		for i, x := range statico.Versions {
			if x == v {
				statico.Versions = append(statico.Versions[:i:i], statico.Versions[i+1:]...)
				break
			}
		}
		delete(statico.Files, v.Name)
		reply(w, 200, map[string]string{"status": "deleted"})
	default:
		reply(w, 405, map[string]string{"message": "method not allowed"})
	}
}

func (s *Server) storage(w http.ResponseWriter, r *http.Request, path string) {
	rest := strings.TrimPrefix(path, "/v2/storages/")
	storage, rest, _ := strings.Cut(rest, "/")
	statico := s.byID(strings.TrimPrefix(storage, "statico-"))
	if statico == nil {
		notFound(w)
		return
	}
	if rest == "archive" && r.Method == "GET" {
		version := r.URL.Query().Get("path")
		files, ok := statico.Files[version]
		if !ok {
			notFound(w)
			return
		}
		names := make([]string, 0, len(files))
		for p := range files {
			names = append(names, p)
		}
		sort.Strings(names)
		if r.URL.Query().Get("format") == "zip" {
			w.Header().Set("Content-Type", "application/zip")
			zw := zip.NewWriter(w)
			for _, n := range names {
				f, _ := zw.Create(n)
				f.Write(files[n])
			}
			zw.Close()
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)
		for _, n := range names {
			tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: n, Mode: 0o644, Size: int64(len(files[n]))})
			tw.Write(files[n])
		}
		tw.Close()
		gz.Close()
		return
	}
	if rest == "archive" && r.Method == "POST" {
		if s.RejectArchives {
			reply(w, 400, map[string]interface{}{"message": "Content-Type must be application/json", "errors": []map[string]string{{"message": "Invalid Content-Type. API only accepts requests with Content-Type: application/json", "code": "INVALID_CONTENT_TYPE"}}})
			return
		}
		version := r.URL.Query().Get("path")
		if _, ok := statico.Files[version]; !ok {
			notFound(w)
			return
		}
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			reply(w, 400, map[string]string{"message": "not a gzip archive"})
			return
		}
		tr := tar.NewReader(gz)
		count := 0
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				reply(w, 400, map[string]string{"message": "bad archive: " + err.Error()})
				return
			}
			if header.Typeflag != tar.TypeReg {
				continue
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				reply(w, 400, map[string]string{"message": "bad archive: " + err.Error()})
				return
			}
			statico.Files[version][header.Name] = data
			count++
		}
		s.Archives++
		s.ArchiveFiles += count
		reply(w, 200, map[string]interface{}{"files": count})
		return
	}
	if rest == "hashes" {
		version := r.URL.Query().Get("path")
		files := statico.Files[version]
		list := []map[string]interface{}{}
		paths := make([]string, 0, len(files))
		for p := range files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			list = append(list, map[string]interface{}{"path": p, "size": len(files[p]), "hash": hash(files[p])})
		}
		reply(w, 200, map[string]interface{}{"files": list})
		return
	}
	if !strings.HasPrefix(rest, "files/") {
		notFound(w)
		return
	}
	segments := strings.Split(strings.TrimPrefix(rest, "files/"), "/")
	for i, seg := range segments {
		if u, err := url.PathUnescape(seg); err == nil {
			segments[i] = u
		}
	}
	version, file := segments[0], strings.Join(segments[1:], "/")
	if _, ok := statico.Files[version]; !ok {
		notFound(w)
		return
	}
	switch r.Method {
	case "PUT":
		data, _ := io.ReadAll(r.Body)
		statico.Files[version][file] = data
		reply(w, 200, map[string]string{"path": file})
	case "GET":
		data, ok := statico.Files[version][file]
		if !ok {
			notFound(w)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	case "DELETE":
		if r.URL.Query().Get("recursive") == "true" {
			removed := 0
			for p := range statico.Files[version] {
				if p == file || strings.HasPrefix(p, file+"/") {
					delete(statico.Files[version], p)
					removed++
				}
			}
			if removed == 0 {
				notFound(w)
				return
			}
			s.RecursiveDeletes++
			reply(w, 200, map[string]string{"status": "deleted"})
			return
		}
		if _, ok := statico.Files[version][file]; !ok {
			notFound(w)
			return
		}
		delete(statico.Files[version], file)
		reply(w, 200, map[string]string{"status": "deleted"})
	default:
		reply(w, 405, map[string]string{"message": "method not allowed"})
	}
}
