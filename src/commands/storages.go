package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"futrou-cli/src/services"
	"futrou-cli/src/utils"

	"github.com/urfave/cli/v2"
)

var storagesCommand = &cli.Command{
	Name:  "storages",
	Usage: "Manage persistent storage",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List storages in a project (defaults to futrou.json, then the login workspace's \"default\" project)",
			Flags: []cli.Flag{
				workspaceFlag,
				projectFlag,
			},
			Action: func(c *cli.Context) error {
				workspaceID, err := resolveWorkspaceID(c)
				if err != nil {
					return err
				}
				projectID, err := resolveProjectID(c, workspaceID)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				result, err := listStoragesForProject(client, workspaceID, projectID)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "projectId", "storagePlanId", "regionId", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a storage by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("storage ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/storages/"+id, nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
		},
		{
			Name:  "create",
			Usage: "Create a storage (defaults to futrou.json, then the login workspace's \"default\" project)",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Required: true, Usage: "Storage name"},
				&cli.StringFlag{Name: "plan", Required: true, Usage: "Storage plan ID"},
				workspaceFlag,
				projectFlag,
			},
			Action: func(c *cli.Context) error {
				workspaceID, err := resolveWorkspaceID(c)
				if err != nil {
					return err
				}
				projectID, err := resolveProjectID(c, workspaceID)
				if err != nil {
					return err
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{
					"name":          c.String("name"),
					"storagePlanId": c.String("plan"),
					"workspaceId":   workspaceID,
					"projectId":     projectID,
				}
				var result interface{}
				status, err := client.RequestInto("POST", "/v2/storages", body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Storage created")
				printJSON(result)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update a storage",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "New name"},
				&cli.StringFlag{Name: "plan", Usage: "New storage plan ID"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("storage ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{}
				if v := c.String("name"); v != "" {
					body["name"] = v
				}
				if v := c.String("plan"); v != "" {
					body["storagePlanId"] = v
				}
				if len(body) == 0 {
					return fmt.Errorf("no fields to update")
				}
				var result interface{}
				status, err := client.RequestInto("PATCH", "/v2/storages/"+id, body, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				fmt.Println("✓ Storage updated")
				return nil
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a storage",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("storage ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("DELETE", "/v2/storages/"+id, nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Storage deleted")
				return nil
			},
		},
		{
			Name:      "instances",
			Usage:     "List storage instances",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("storage ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/storages/"+id+"/instances", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "status", "createdAt"})
				return nil
			},
		},
		{
			Name:      "logs",
			Usage:     "View storage logs",
			ArgsUsage: "<id>",
			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "tail", Usage: "Show only recent logs"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("storage ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				path := "/v2/storages/" + id + "/logs"
				if c.Bool("tail") {
					path += "/tail"
				}
				var result interface{}
				status, err := client.RequestInto("GET", path, nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
		},
		storagePlansCommand,
		storageFilesCommand,
		storageSyncCommand,
	},
}

var storagePlansCommand = &cli.Command{
	Name:  "plans",
	Usage: "Manage storage plans",
	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "List all storage plans",
			Action: func(c *cli.Context) error {
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/storages/plans", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(result)
				}
				printTable(result, []string{"id", "name", "displayName", "isPublic", "createdAt"})
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Get a storage plan by ID",
			ArgsUsage: "<id>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("storage plan ID required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", "/v2/storages/plans/"+id, nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
		},
	},
}

// listStoragesForProject lists storages scoped to a single workspace and
// project. GET /v2/storages requires at least one of workspaceId/projectId
// to be given, unlike most other list endpoints.
func listStoragesForProject(client *services.ApiClient, workspaceID, projectID string) ([]interface{}, error) {
	var result []interface{}
	status, err := client.RequestInto("GET", "/v2/storages?workspaceId="+workspaceID+"&projectId="+projectID, nil, &result)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("request failed with status %d", status)
	}
	return result, nil
}

// storagePath joins a storage ID and an in-storage file path into the
// wildcard route used by the files API, e.g. /v2/storages/<id>/files/<path>.
func storagePath(base, id, filePath string) string {
	filePath = strings.TrimPrefix(filePath, "/")
	if filePath == "" {
		return "/v2/storages/" + id + "/" + base
	}
	return "/v2/storages/" + id + "/" + base + "/" + filePath
}

// storageFileEntry mirrors one entry returned by GET /v2/storages/<id>/files
// (and the single-path info variant): {name, path, isDir, size, contentType,
// modifiedAt, etag}.
type storageFileEntry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	IsDir       bool      `json:"isDir"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType,omitempty"`
	ModifiedAt  time.Time `json:"modifiedAt"`
	ETag        string    `json:"etag,omitempty"`
}

type storageFileList struct {
	Entries []storageFileEntry `json:"entries"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
	Path    string             `json:"path"`
	Total   int                `json:"total"`
}

const storageListPageSize = 200

// listStorageFilesRecursive lists every file (not directories) under
// remotePath in storage id, paging through the API and descending into
// subdirectories. Returned entry paths are relative to the storage root.
func listStorageFilesRecursive(client *services.ApiClient, id, remotePath string) ([]storageFileEntry, error) {
	var files []storageFileEntry
	dirs := []string{remotePath}
	seen := map[string]bool{}
	for len(dirs) > 0 {
		dir := dirs[0]
		dirs = dirs[1:]
		if seen[dir] {
			continue
		}
		seen[dir] = true

		offset := 0
		for {
			requestPath := storagePath("files", id, dir)
			requestPath += fmt.Sprintf("?limit=%d&offset=%d", storageListPageSize, offset)
			var page storageFileList
			status, err := client.RequestInto("GET", requestPath, nil, &page)
			if err != nil {
				return nil, err
			}
			if status >= 400 {
				return nil, fmt.Errorf("listing %s: request failed with status %d", dir, status)
			}
			for _, entry := range page.Entries {
				if entry.IsDir {
					dirs = append(dirs, entry.Path)
					continue
				}
				files = append(files, entry)
			}
			offset += len(page.Entries)
			if len(page.Entries) == 0 || offset >= page.Total {
				break
			}
		}
	}
	return files, nil
}

// resolveStorageTargets expands a glob pattern into the file entries it
// matches, by listing a full recursive directory tree rooted at the
// pattern's non-glob prefix and filtering client-side. Callers are expected
// to have already handled the non-glob (exact path) case themselves.
func resolveStorageTargets(client *services.ApiClient, id, target string) ([]storageFileEntry, error) {
	target = strings.TrimPrefix(target, "/")
	root := globRoot(target)
	all, err := listStorageFilesRecursive(client, id, root)
	if err != nil {
		return nil, err
	}
	matched := make([]storageFileEntry, 0, len(all))
	for _, entry := range all {
		if utils.GlobMatch(target, entry.Path) {
			matched = append(matched, entry)
		}
	}
	return matched, nil
}

// globRoot returns the longest path prefix of pattern that contains no glob
// metacharacters, used to scope a remote listing before client-side
// filtering instead of always walking the whole storage. A leading '/' is
// preserved so absolute local paths stay absolute.
func globRoot(pattern string) string {
	absolute := strings.HasPrefix(pattern, "/")
	segments := strings.Split(strings.Trim(pattern, "/"), "/")
	root := make([]string, 0, len(segments))
	for _, segment := range segments {
		if utils.HasGlobMeta(segment) {
			break
		}
		root = append(root, segment)
	}
	joined := strings.Join(root, "/")
	if absolute {
		return "/" + joined
	}
	return joined
}

// localFile describes one local file discovered by resolveLocalTargets,
// with a path relative to the resolution root for mapping to remote paths.
type localFile struct {
	AbsPath string
	RelPath string
	Info    os.FileInfo
}

// resolveLocalTargets expands a local path argument (file, directory, or
// glob) into the concrete files it selects, with slash-separated relative
// paths suitable for joining onto a remote prefix.
func resolveLocalTargets(target string) ([]localFile, error) {
	target = filepath.Clean(target)
	if !utils.HasGlobMeta(target) {
		info, err := os.Stat(target)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return []localFile{{AbsPath: target, RelPath: filepath.Base(target), Info: info}}, nil
		}
		return walkLocalDir(target, "")
	}

	root := globRoot(filepath.ToSlash(target))
	if root == "" {
		root = "."
	}
	var matched []localFile
	err := filepath.Walk(root, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		slashPath := filepath.ToSlash(walkPath)
		if utils.GlobMatch(target, slashPath) {
			rel, err := filepath.Rel(root, walkPath)
			if err != nil {
				rel = walkPath
			}
			matched = append(matched, localFile{AbsPath: walkPath, RelPath: filepath.ToSlash(rel), Info: info})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matched, nil
}

func walkLocalDir(root, _ string) ([]localFile, error) {
	var files []localFile
	err := filepath.Walk(root, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, walkPath)
		if err != nil {
			rel = walkPath
		}
		files = append(files, localFile{AbsPath: walkPath, RelPath: filepath.ToSlash(rel), Info: info})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func uploadOneFile(client *services.ApiClient, id, localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", localPath, err)
	}
	defer file.Close()
	_, status, err := client.UploadFile(storagePath("files", id, remotePath), "application/octet-stream", file)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("uploading %s: request failed with status %d", remotePath, status)
	}
	return nil
}

func downloadOneFile(client *services.ApiClient, id, remotePath, localPath string) error {
	data, _, status, err := client.DownloadFile(storagePath("files", id, remotePath))
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("downloading %s: request failed with status %d", remotePath, status)
	}
	if dir := filepath.Dir(localPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", localPath, err)
	}
	return nil
}

// syncSide is one side of a sync comparison: a file that exists locally,
// remotely, or both, keyed by its path relative to the sync roots.
type syncSide struct {
	relPath string
	local   *localFile
	remote  *storageFileEntry
}

var storageSyncCommand = &cli.Command{
	Name:      "sync",
	Usage:     "Two-way sync between a local directory and a storage path by modification time",
	ArgsUsage: "<storage-id> [remote-path] [local-dir]",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "dry-run", Usage: "Show what would change without transferring anything"},
	},
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		remoteDir := c.Args().Get(1)
		localDir := c.Args().Get(2)
		if localDir == "" {
			localDir = "./"
		}
		if id == "" {
			return fmt.Errorf("storage ID required")
		}
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		dryRun := c.Bool("dry-run")

		if info, err := os.Stat(localDir); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(localDir, 0755); err != nil {
				return fmt.Errorf("creating %s: %w", localDir, err)
			}
		} else if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", localDir)
		}

		localFiles, err := walkLocalDir(localDir, "")
		if err != nil {
			return fmt.Errorf("walking %s: %w", localDir, err)
		}
		remoteFiles, err := listStorageFilesRecursive(client, id, strings.TrimPrefix(remoteDir, "/"))
		if err != nil {
			return fmt.Errorf("listing %s: %w", remoteDir, err)
		}
		remoteRoot := strings.TrimPrefix(remoteDir, "/")

		sides := map[string]*syncSide{}
		for i := range localFiles {
			f := localFiles[i]
			side := sides[f.RelPath]
			if side == nil {
				side = &syncSide{relPath: f.RelPath}
				sides[f.RelPath] = side
			}
			side.local = &f
		}
		for i := range remoteFiles {
			entry := remoteFiles[i]
			rel := strings.TrimPrefix(strings.TrimPrefix(entry.Path, remoteRoot), "/")
			side := sides[rel]
			if side == nil {
				side = &syncSide{relPath: rel}
				sides[rel] = side
			}
			side.remote = &entry
		}

		var uploaded, downloaded, skipped int
		for _, side := range sides {
			remotePath := strings.TrimSuffix(remoteRoot, "/") + "/" + side.relPath
			localPath := filepath.Join(localDir, filepath.FromSlash(side.relPath))

			switch {
			case side.local != nil && side.remote == nil:
				fmt.Printf("↑ %s (new locally)\n", side.relPath)
				uploaded++
				if !dryRun {
					if err := uploadOneFile(client, id, side.local.AbsPath, remotePath); err != nil {
						return fmt.Errorf("%s: %w", side.relPath, err)
					}
				}
			case side.remote != nil && side.local == nil:
				fmt.Printf("↓ %s (new remotely)\n", side.relPath)
				downloaded++
				if !dryRun {
					if err := downloadOneFile(client, id, side.remote.Path, localPath); err != nil {
						return fmt.Errorf("%s: %w", side.relPath, err)
					}
				}
			case side.local != nil && side.remote != nil:
				localMTime := side.local.Info.ModTime().UTC()
				remoteMTime := side.remote.ModifiedAt.UTC()
				diff := localMTime.Sub(remoteMTime)
				switch {
				case diff > time.Second:
					fmt.Printf("↑ %s (local newer)\n", side.relPath)
					uploaded++
					if !dryRun {
						if err := uploadOneFile(client, id, side.local.AbsPath, remotePath); err != nil {
							return fmt.Errorf("%s: %w", side.relPath, err)
						}
					}
				case diff < -time.Second:
					fmt.Printf("↓ %s (remote newer)\n", side.relPath)
					downloaded++
					if !dryRun {
						if err := downloadOneFile(client, id, side.remote.Path, localPath); err != nil {
							return fmt.Errorf("%s: %w", side.relPath, err)
						}
					}
				default:
					skipped++
				}
			}
		}

		verb := "Synced"
		if dryRun {
			verb = "Would sync"
		}
		fmt.Printf("✓ %s: %d uploaded, %d downloaded, %d up to date\n", verb, uploaded, downloaded, skipped)
		return nil
	},
}

var storageFilesCommand = &cli.Command{
	Name:  "files",
	Usage: "Manage files within a storage",
	Subcommands: []*cli.Command{
		{
			Name:      "list",
			Usage:     "List or search files in a storage; the path may be a glob (e.g. images/*, **/*.log)",
			ArgsUsage: "<storage-id> [path-or-glob]",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "query", Usage: "Search query"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				if id == "" {
					return fmt.Errorf("storage ID required")
				}
				target := c.Args().Get(1)
				client, err := requireAuth(c)
				if err != nil {
					return err
				}

				if q := c.String("query"); q != "" {
					requestPath := storagePath("files", id, target) + "?query=" + q
					var result interface{}
					status, err := client.RequestInto("GET", requestPath, nil, &result)
					if err != nil {
						return err
					}
					if status >= 400 {
						return fmt.Errorf("request failed with status %d", status)
					}
					if isJSON(c) {
						return printJSON(result)
					}
					printTable(result, []string{"name", "path", "size", "modifiedAt"})
					return nil
				}

				if target == "" || !utils.HasGlobMeta(target) {
					requestPath := storagePath("files", id, target)
					var result interface{}
					status, err := client.RequestInto("GET", requestPath, nil, &result)
					if err != nil {
						return err
					}
					if status >= 400 {
						return fmt.Errorf("request failed with status %d", status)
					}
					if isJSON(c) {
						return printJSON(result)
					}
					printTable(result, []string{"name", "path", "size", "modifiedAt"})
					return nil
				}

				matches, err := resolveStorageTargets(client, id, target)
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(matches)
				}
				printTable(matches, []string{"name", "path", "size", "modifiedAt"})
				return nil
			},
		},
		{
			Name:      "info",
			Usage:     "Inspect file or folder metadata",
			ArgsUsage: "<storage-id> <path>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				filePath := c.Args().Get(1)
				if id == "" || filePath == "" {
					return fmt.Errorf("storage ID and path required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				var result interface{}
				status, err := client.RequestInto("GET", storagePath("files", id, filePath)+"?info=true", nil, &result)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				return printJSON(result)
			},
		},
		{
			Name:      "download",
			Usage:     "Download a file, folder, or glob match from a storage",
			ArgsUsage: "<storage-id> <remote-path-or-glob> <local-path>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				remoteTarget := c.Args().Get(1)
				localPath := c.Args().Get(2)
				if id == "" || remoteTarget == "" || localPath == "" {
					return fmt.Errorf("storage ID, remote path, and local path required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}

				if !utils.HasGlobMeta(remoteTarget) {
					var info storageFileEntry
					status, err := client.RequestInto("GET", storagePath("files", id, remoteTarget)+"?info=true", nil, &info)
					if err != nil {
						return err
					}
					if status >= 400 {
						return fmt.Errorf("request failed with status %d", status)
					}
					if !info.IsDir {
						if err := downloadOneFile(client, id, remoteTarget, localPath); err != nil {
							return err
						}
						fmt.Printf("✓ Downloaded to %s\n", localPath)
						return nil
					}
					// remoteTarget is a directory: fall through to recursive download.
				}

				matches, err := resolveStorageTargets(client, id, remoteTarget)
				if err != nil {
					return err
				}
				if len(matches) == 0 {
					return fmt.Errorf("no files matched %q", remoteTarget)
				}
				root := globRoot(strings.TrimPrefix(remoteTarget, "/"))
				count := 0
				for _, entry := range matches {
					rel := strings.TrimPrefix(strings.TrimPrefix(entry.Path, root), "/")
					dest := filepath.Join(localPath, filepath.FromSlash(rel))
					if err := downloadOneFile(client, id, entry.Path, dest); err != nil {
						return fmt.Errorf("%s: %w", entry.Path, err)
					}
					count++
				}
				fmt.Printf("✓ Downloaded %d file(s) to %s\n", count, localPath)
				return nil
			},
		},
		{
			Name:      "upload",
			Usage:     "Upload a file, folder, or glob match to a storage",
			ArgsUsage: "<storage-id> <local-path-or-glob> <remote-path>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				localTarget := c.Args().Get(1)
				remotePath := c.Args().Get(2)
				if id == "" || localTarget == "" || remotePath == "" {
					return fmt.Errorf("storage ID, local path, and remote path required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}

				info, statErr := os.Stat(localTarget)
				if statErr == nil && !info.IsDir() && !utils.HasGlobMeta(localTarget) {
					if err := uploadOneFile(client, id, localTarget, remotePath); err != nil {
						return err
					}
					fmt.Println("✓ File uploaded")
					return nil
				}

				files, err := resolveLocalTargets(localTarget)
				if err != nil {
					return err
				}
				if len(files) == 0 {
					return fmt.Errorf("no local files matched %q", localTarget)
				}
				for _, f := range files {
					dest := strings.TrimSuffix(remotePath, "/") + "/" + f.RelPath
					if err := uploadOneFile(client, id, f.AbsPath, dest); err != nil {
						return fmt.Errorf("%s: %w", f.AbsPath, err)
					}
				}
				fmt.Printf("✓ Uploaded %d file(s)\n", len(files))
				return nil
			},
		},
		{
			Name:      "mkdir",
			Usage:     "Create a folder in a storage",
			ArgsUsage: "<storage-id> <path>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				dirPath := c.Args().Get(1)
				if id == "" || dirPath == "" {
					return fmt.Errorf("storage ID and path required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				status, err := client.RequestInto("POST", storagePath("files/mkdir", id, dirPath), nil, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				fmt.Println("✓ Folder created")
				return nil
			},
		},
		{
			Name:      "move",
			Usage:     "Move or rename a file or folder in a storage",
			ArgsUsage: "<storage-id> <path> <destination>",
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				srcPath := c.Args().Get(1)
				dest := c.Args().Get(2)
				if id == "" || srcPath == "" || dest == "" {
					return fmt.Errorf("storage ID, path, and destination required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}
				body := map[string]interface{}{"dest": dest}
				status, err := client.RequestInto("POST", storagePath("files/move", id, srcPath), body, nil)
				if err != nil {
					return err
				}
				if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				fmt.Println("✓ Moved")
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a file, folder, or glob match in a storage",
			ArgsUsage: "<storage-id> <path-or-glob>",
			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "recursive", Usage: "Delete folders recursively"},
			},
			Action: func(c *cli.Context) error {
				id := c.Args().First()
				target := c.Args().Get(1)
				if id == "" || target == "" {
					return fmt.Errorf("storage ID and path required")
				}
				client, err := requireAuth(c)
				if err != nil {
					return err
				}

				if !utils.HasGlobMeta(target) {
					requestPath := storagePath("files", id, target)
					if c.Bool("recursive") {
						requestPath += "?recursive=true"
					}
					status, err := client.RequestInto("DELETE", requestPath, nil, nil)
					if err != nil {
						return err
					}
					if status >= 400 {
						return fmt.Errorf("request failed with status %d", status)
					}
					fmt.Println("✓ Deleted")
					return nil
				}

				matches, err := resolveStorageTargets(client, id, target)
				if err != nil {
					return err
				}
				if len(matches) == 0 {
					return fmt.Errorf("no files matched %q", target)
				}
				for _, entry := range matches {
					status, err := client.RequestInto("DELETE", storagePath("files", id, entry.Path), nil, nil)
					if err != nil {
						return err
					}
					if status >= 400 {
						return fmt.Errorf("deleting %s: request failed with status %d", entry.Path, status)
					}
				}
				fmt.Printf("✓ Deleted %d file(s)\n", len(matches))
				return nil
			},
		},
	},
}
