package statico

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveFormat is the archive format a file name asks for: zip for .zip,
// tar.gz otherwise.
func ArchiveFormat(file string) string {
	if strings.HasSuffix(strings.ToLower(file), ".zip") {
		return "zip"
	}
	return "tar.gz"
}

// ArchiveName is the default file name of a downloaded version.
func ArchiveName(st Statico, version string) string {
	return unsafeName.ReplaceAllString(st.Name, "_") + "-" + version + ".tar.gz"
}

// DownloadArchive downloads a version (empty or "active" is the served one) as
// one archive, in the format of file's extension (.zip, else .tar.gz). The
// archive is written to a temporary file next to file and renamed when whole, so
// a failed download leaves nothing behind. It returns the version's name.
func DownloadArchive(c Client, st Statico, version, file string) (string, error) {
	if version == "" {
		version = "active"
	}
	v, err := GetStaticoVersion(c, st, version)
	if err != nil {
		return "", err
	}
	if file == "" {
		file = ArchiveName(st, v.Name)
	}
	if info, err := os.Stat(file); err == nil && info.IsDir() {
		file = filepath.Join(file, ArchiveName(st, v.Name))
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", filepath.Dir(file), unwrapPathError(err))
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".download-")
	if err != nil {
		return "", fmt.Errorf("cannot write in %s: %w", filepath.Dir(file), unwrapPathError(err))
	}
	defer os.Remove(tmp.Name())
	path := "/v2/storages/" + url.PathEscape(v.storage(st)) + "/archive?path=" + url.QueryEscape(v.Name) + "&format=" + ArchiveFormat(file)
	status, err := c.Stream(path, tmp)
	if cerr := tmp.Close(); err == nil && cerr != nil {
		err = cerr
	}
	if err := failed("downloading version "+v.Name, status, err); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", file, unwrapPathError(err))
	}
	return v.Name, nil
}

// ActivateVersion serves the named version, an older completed one being a rollback.
func ActivateVersion(c Client, st Statico, version string) error {
	status, err := c.RequestInto("POST", st.Path()+"/versions/"+url.PathEscape(version)+"/activate", nil, &struct{}{})
	return failed("activating version "+version, status, err)
}

// VersionExists reports whether the website has a version of that name.
func VersionExists(c Client, st Statico, name string) (bool, error) {
	var v StaticoVersion
	status, err := c.RequestInto("GET", st.Path()+"/versions/"+url.PathEscape(name), nil, &v)
	if versionMissing(status, err) {
		return false, nil
	}
	return v.Name != "", failed("reading version "+name, status, err)
}
