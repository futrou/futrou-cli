package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"futrou-cli/src/constants"
)

// BuildDir is <base>/.futrou, the folder next to a config file where the CLI
// keeps what it stages and caches. It is created only when something needs it,
// with a .gitignore that ignores everything in it.
func BuildDir(base string) string { return filepath.Join(base, constants.BuildDir) }

// CacheDir is <base>/.futrou/cache, where hashes and the record of finished
// installs live. Unlike the rest of .futrou it survives from run to run.
func CacheDir(base string) string { return filepath.Join(base, filepath.FromSlash(constants.CacheDir)) }

// StaticoDir is <base>/.futrou/statico, where the files of an upload are staged.
func StaticoDir(base string) string {
	return filepath.Join(base, filepath.FromSlash(constants.StaticoDir))
}

// TmpDir is <base>/.futrou/tmp, for scratch files that a run does not keep.
func TmpDir(base string) string { return filepath.Join(base, filepath.FromSlash(constants.TmpDir)) }

// DebugDir is <base>/.futrou/debug, where a run writes what helps to debug it.
func DebugDir(base string) string { return filepath.Join(base, filepath.FromSlash(constants.DebugDir)) }

// MakeCacheDir creates <base>/.futrou/cache and, when .futrou is new, its
// .gitignore. It returns the cache folder.
func MakeCacheDir(base string) (string, error) {
	work := BuildDir(base)
	_, statErr := os.Stat(work)
	dir := CacheDir(base)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if os.IsNotExist(statErr) {
		_ = os.WriteFile(filepath.Join(work, ".gitignore"), []byte("*\n"), 0o644)
	}
	return dir, nil
}

func doneFile(base string) string { return filepath.Join(CacheDir(base), "done-commands.json") }

func readDone(base string) map[string]string {
	done := map[string]string{}
	if data, err := os.ReadFile(doneFile(base)); err == nil {
		_ = json.Unmarshal(data, &done) // a damaged file means nothing was done
	}
	return done
}

// CommandDone reports whether a one-time command (an install) already ran
// successfully for the config folder base, under key.
func CommandDone(base, key string) bool {
	_, ok := readDone(base)[key]
	return ok
}

// MarkCommandDone records that the command under key ran successfully. Failing
// to write is not an error: the command then simply runs again next time.
func MarkCommandDone(base, key string) {
	done := readDone(base)
	done[key] = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(done)
	if err != nil {
		return
	}
	dir, err := MakeCacheDir(base)
	if err != nil {
		return
	}
	WriteFileAtomic(filepath.Join(dir, "done-commands.json"), data)
}

// WriteFileAtomic writes data to path through a temporary file, so a reader
// never sees half of it. Failing is silent: it is for caches.
func WriteFileAtomic(path string, data []byte) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return
	}
	if tmp.Close() != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
	}
}
