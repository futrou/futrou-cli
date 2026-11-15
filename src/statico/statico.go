package statico

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"futrou-cli/src/utils"
)

// Client is the part of the Futrou API client the statico code needs. Every
// request, including the file transfers, carries the CLI's own API token: Cloud
// authorises it and forwards the work to the node with its own credentials.
type Client interface {
	RequestInto(method, path string, body interface{}, v interface{}) (int, error)
	// SendBody sends a file, or an archive of many, as the body of a request.
	SendBody(method, path, contentType string, body io.Reader) (interface{}, int, error)
	// Stream copies the body of a GET to w as it arrives.
	Stream(path string, w io.Writer) (int, error)
}

// NotFoundError is returned by FindStatico when no website has that name or ID.
type NotFoundError struct{ Ref string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("statico website %q not found", e.Ref) }

// Scope narrows a lookup by name to one workspace and project.
type Scope struct {
	WorkspaceID string
	ProjectID   string
}

func (s Scope) query() string {
	values := url.Values{}
	if s.WorkspaceID != "" {
		values.Set("workspaceId", s.WorkspaceID)
	}
	if s.ProjectID != "" {
		values.Set("projectId", s.ProjectID)
	}
	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

// Statico is a statico website.
type Statico struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Storage       string `json:"storage"`
	ActiveVersion string `json:"activeVersion"`
	// PublicDomain is where the website is served, such as statico-f372395487bc.futrou.cloud.
	PublicDomain string `json:"publicDomain"`
}

// URL is the address of the website, or "" when the API did not say.
func (s Statico) URL() string {
	if s.PublicDomain == "" {
		return ""
	}
	return "https://" + s.PublicDomain
}

// StorageID is the storage that holds the website's files.
func (s Statico) StorageID() string {
	if s.Storage != "" {
		return s.Storage
	}
	return "statico-" + s.ID
}

// Path is the website's API path.
func (s Statico) Path() string { return "/v2/staticos/" + url.PathEscape(s.ID) }

// Version is one version of a website, a folder of its storage.
type StaticoVersion struct {
	Name    string `json:"name"`
	Number  int    `json:"number"`
	Storage string `json:"storage"`
	Status  string `json:"status"`
	Error   string `json:"error"`
	Files   int    `json:"files"`
	Size    int64  `json:"size"`
}

// versionMissing reports whether a version request failed because the version
// does not exist (a website that was never deployed has no active version). The
// API answers that with 404, or with another 4xx whose message says so.
func versionMissing(status int, err error) bool {
	if status == 404 {
		return true
	}
	return status >= 400 && status < 500 && err != nil && strings.Contains(strings.ToLower(err.Error()), "version not found")
}

func failed(op string, status int, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if status >= 400 {
		return fmt.Errorf("%s: request failed with status %d", op, status)
	}
	return nil
}

// ListStaticos returns the websites visible in scope.
func ListStaticos(c Client, scope Scope) ([]Statico, error) {
	var staticos []Statico
	status, err := c.RequestInto("GET", "/v2/staticos"+scope.query(), nil, &staticos)
	if err := failed("listing statico websites", status, err); err != nil {
		return nil, err
	}
	return staticos, nil
}

// FindStatico returns the website whose ID or name is ref. A name that more than
// one website in scope has is refused, so nothing is uploaded to the wrong one.
func FindStatico(c Client, ref string, scope Scope) (*Statico, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("statico website name or ID required")
	}
	if utils.IsUuid(ref) {
		var statico Statico
		status, err := c.RequestInto("GET", "/v2/staticos/"+url.PathEscape(ref), nil, &statico)
		if err == nil && status < 400 && statico.ID != "" {
			return &statico, nil
		}
	}
	staticos, err := ListStaticos(c, scope)
	if err != nil {
		return nil, err
	}
	var matches []Statico
	for _, statico := range staticos {
		if statico.ID == ref {
			return &statico, nil
		}
		if statico.Name == ref {
			matches = append(matches, statico)
		}
	}
	switch len(matches) {
	case 0:
		return nil, &NotFoundError{Ref: ref}
	case 1:
		return &matches[0], nil
	}
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.ID
	}
	return nil, fmt.Errorf("%d statico websites are named %q (%s): use the ID or --workspace/--project", len(matches), ref, strings.Join(ids, ", "))
}

// Versions lists a website's versions.
func ListStaticoVersions(c Client, statico Statico) ([]StaticoVersion, error) {
	var raw interface{}
	status, err := c.RequestInto("GET", statico.Path()+"/versions", nil, &raw)
	if err := failed("listing versions", status, err); err != nil {
		return nil, err
	}
	var versions []StaticoVersion
	if err := decode(raw, &versions); err != nil {
		var wrapped struct {
			Items []StaticoVersion `json:"items"`
		}
		if err := decode(raw, &wrapped); err != nil {
			return nil, fmt.Errorf("reading versions: %w", err)
		}
		versions = wrapped.Items
	}
	return versions, nil
}

// GetStaticoVersion reads one version; the name "active" is the served one.
func GetStaticoVersion(c Client, statico Statico, name string) (*StaticoVersion, error) {
	var v StaticoVersion
	status, err := c.RequestInto("GET", statico.Path()+"/versions/"+url.PathEscape(name), nil, &v)
	if err := failed("reading version "+name, status, err); err != nil {
		return nil, err
	}
	if v.Name == "" {
		v.Name = name
	}
	return &v, nil
}

func (v StaticoVersion) storage(statico Statico) string {
	if v.Storage != "" {
		return v.Storage
	}
	return statico.StorageID()
}

// filePath is the storage files route for a file of the version.
func filePath(storage, version, rel string) string {
	segments := []string{version}
	if rel != "" {
		segments = append(segments, strings.Split(rel, "/")...)
	}
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return "/v2/storages/" + url.PathEscape(storage) + "/files/" + strings.Join(segments, "/")
}

// RemoteFile is a file of a version with the hash its storage computed.
type RemoteFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

// Hashes lists the files of a version with their sha256, for comparing.
func Hashes(c Client, statico Statico, v StaticoVersion) ([]RemoteFile, error) {
	var out struct {
		Files []RemoteFile `json:"files"`
	}
	path := "/v2/storages/" + url.PathEscape(v.storage(statico)) + "/hashes?path=" + url.QueryEscape(v.Name)
	status, err := c.RequestInto("GET", path, nil, &out)
	if err := failed("listing the files of version "+v.Name, status, err); err != nil {
		return nil, err
	}
	return out.Files, nil
}
