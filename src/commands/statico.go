package commands

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	projectconfig "futrou-cli/src/config"
	"futrou-cli/src/services"
	"futrou-cli/src/statico"
	"futrou-cli/src/utils"

	"github.com/urfave/cli/v2"
)

var staticoIncludeFlag = &cli.StringSliceFlag{Name: "include", Aliases: []string{"i"}, Usage: "File, folder or glob to include: SOURCE or SOURCE:DESTINATION, relative to the current directory or absolute (repeatable, in order; a folder's files land at the root, a file or glob keeps its path, SOURCE:DEST puts them in DEST, {js,css} and ** work)"}
var staticoExcludeFlag = &cli.StringSliceFlag{Name: "exclude", Aliases: []string{"e"}, Usage: "File, folder or glob to leave out of what the includes selected (repeatable; the same as SOURCE: false in futrou.json, applied after the includes)"}

// staticoSelectFlags choose the local files of upload and sync.
var staticoSelectFlags = []cli.Flag{staticoIncludeFlag, staticoExcludeFlag}

var staticoVersionFlags = []cli.Flag{
	&cli.StringFlag{Name: "version", Usage: "Name of the version to create (default: the current time); an existing version of that name is activated instead, without an upload"},
	&cli.BoolFlag{Name: "empty", Usage: "Start the version with no files instead of a copy of the served one"},
	&cli.BoolFlag{Name: "activate", Value: true, Usage: "Serve the new version; --activate=false leaves it ready but not served"},
	&cli.BoolFlag{Name: "html-to-folders", Value: true, Usage: "Also copy page.html as page/index.html; --html-to-folders=false does not"},
	&cli.StringFlag{Name: "default-file", Usage: "File served for a folder (default /index.html)"},
	&cli.IntFlag{Name: "default-status", Usage: "Status of a default file answer (default 200)"},
	&cli.StringFlag{Name: "fallback-file", Usage: "File served for a path that is not found (default /404.html)"},
	&cli.IntFlag{Name: "fallback-status", Usage: "Status of a fallback answer (default 404)"},
	&cli.BoolFlag{Name: "dry-run", Usage: "Show what would change without sending anything"},
}

// staticoCreateFlags apply when an upload creates the website, which it does on the first upload.
var staticoCreateFlags = []cli.Flag{
	&cli.StringFlag{Name: "region", Usage: "Region name or ID of a website the upload creates"},
	&cli.BoolFlag{Name: "public", Value: true, Usage: "Anybody can read a website the upload creates; --public=false answers only to a request with an API token"},
}

var staticoCommand = &cli.Command{
	Name:  "statico",
	Usage: "Manage static websites",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List statico websites (defaults to all you can see; --workspace/--project or futrou.json narrow it)",
			Flags: []cli.Flag{workspaceFlag, projectFlag},
			Action: func(c *cli.Context) error {
				client, scope, err := staticoSession(c)
				if err != nil {
					return err
				}
				var staticos []map[string]interface{}
				if status, err := client.RequestInto("GET", "/v2/staticos"+scopeQuery(scope), nil, &staticos); err != nil {
					return err
				} else if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(staticos)
				}
				printTable(staticos, []string{"id", "name", "projectId", "regionId", "public", "createdAt"})
				return nil
			},
		},
		{
			Name:      "show",
			Usage:     "Show a statico website by name or ID, with its versions",
			ArgsUsage: "<name|id>",
			Flags:     []cli.Flag{workspaceFlag, projectFlag},
			Action: func(c *cli.Context) error {
				client, st, err := staticoTarget(c)
				if err != nil {
					return err
				}
				var detail map[string]interface{}
				if status, err := client.RequestInto("GET", st.Path(), nil, &detail); err != nil {
					return err
				} else if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if versions, err := statico.ListStaticoVersions(client, *st); err == nil {
					detail["versions"] = versions
				}
				return printJSON(detail)
			},
		},
		{
			Name:      "rm",
			Usage:     "Delete a statico website with all its versions and files",
			ArgsUsage: "<name|id>",
			Flags:     []cli.Flag{workspaceFlag, projectFlag},
			Action: func(c *cli.Context) error {
				client, st, err := staticoTarget(c)
				if err != nil {
					return err
				}
				if status, err := client.RequestInto("DELETE", st.Path(), nil, nil); err != nil {
					return err
				} else if status >= 400 {
					return fmt.Errorf("request failed with status %d", status)
				}
				if isJSON(c) {
					return printJSON(map[string]string{"status": "deleted"})
				}
				fmt.Println("✓ Statico website deleted")
				return nil
			},
		},
		{
			Name:      "deploy",
			Aliases:   []string{"up"},
			Usage:     "Upload files as a new version of a statico website and serve it (the first deploy creates the website; the name defaults to the folder)",
			ArgsUsage: "[name|id]",
			Description: "Selects the files with --include (SOURCE or SOURCE:DESTINATION; files, folders and globs relative to the\n" +
				"   current directory, in order) minus --exclude, sends the ones that differ\n" +
				"   from the served version and serves the result with no downtime. Nothing happens when the served version\n" +
				"   already has these files. Without an include a website declared in\n" +
				"   futrou.json uses its own, else the deploy stops with an error. Without a name the website declared in\n" +
				"   futrou.json is used (when it declares one), else the name of the current folder.\n" +
				"   A folder's files land at the root of the website; a file or glob keeps its path (SOURCE:DESTINATION moves it). A _redirects file at the\n" +
				"   root of the website (Cloudflare Pages syntax) sets the redirects of the version and is not uploaded.\n" +
				"   --version names the version; an existing version of that name is served instead, without an upload.\n\n" +
				"   Example: futrou statico deploy --include ./dist --exclude '*.map' docs",
			Flags: append(append(append([]cli.Flag{workspaceFlag, projectFlag}, staticoSelectFlags...), staticoVersionFlags...), staticoCreateFlags...),
			Action: func(c *cli.Context) error {
				return runStaticoDeploy(c)
			},
		},
		{
			Name:      "version",
			Usage:     "List and serve the versions of a statico website",
			ArgsUsage: "[name|id]",
			// without a subcommand it lists, so `statico versions docs` keeps working
			Flags:  []cli.Flag{workspaceFlag, projectFlag},
			Action: staticoVersionsAction,
			Subcommands: []*cli.Command{
				{
					Name:      "ls",
					Usage:     "List all the versions of a statico website, newest first",
					ArgsUsage: "[name|id]",
					Flags:     []cli.Flag{workspaceFlag, projectFlag},
					Action:    staticoVersionsAction,
				},
				{
					Name:      "activate",
					Usage:     "Serve a version of a statico website (an older one is a rollback)",
					ArgsUsage: "[name|id]",
					Flags:     staticoActivateFlags,
					Action:    staticoActivateAction,
				},
			},
		},
		{
			// the older spelling of `version activate`
			Name:      "activate",
			Hidden:    true,
			Usage:     "Serve a version of a statico website (an older one is a rollback)",
			ArgsUsage: "[name|id]",
			Flags:     staticoActivateFlags,
			Action:    staticoActivateAction,
		},
		{
			Name:      "download",
			Usage:     "Download a version of a statico website (the served one by default) as <name>-<version>.tar.gz",
			ArgsUsage: "[name|id]",
			Flags: []cli.Flag{workspaceFlag, projectFlag,
				&cli.StringFlag{Name: "version", Usage: "Version to download (default: the served one)"}},
			Action: func(c *cli.Context) error {
				client, st, err := staticoTarget(c)
				if err != nil {
					return err
				}
				version, err := statico.DownloadArchive(client, *st, c.String("version"), "")
				if err != nil {
					return err
				}
				if isJSON(c) {
					return printJSON(map[string]string{"version": version, "file": statico.ArchiveName(*st, version)})
				}
				fmt.Printf("✓ Downloaded version %s of %s to %s\n", version, st.Name, statico.ArchiveName(*st, version))
				return nil
			},
		},
	},
}

var staticoActivateFlags = []cli.Flag{workspaceFlag, projectFlag,
	&cli.StringFlag{Name: "version", Required: true, Usage: "Name of the version to serve (see: futrou statico version ls)"}}

func staticoActivateAction(c *cli.Context) error {
	client, st, err := staticoTarget(c)
	if err != nil {
		return err
	}
	version := c.String("version")
	if err := statico.ActivateVersion(client, *st, version); err != nil {
		return err
	}
	if isJSON(c) {
		return printJSON(map[string]string{"version": version, "url": st.URL()})
	}
	fmt.Printf("✓ %s: version %s is live%s\n", st.Name, version, liveAt(st.URL()))
	return nil
}

func staticoVersionsAction(c *cli.Context) error {
	client, st, err := staticoTarget(c)
	if err != nil {
		return err
	}
	versions, err := statico.ListStaticoVersions(client, *st)
	if err != nil {
		return err
	}
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].Number > versions[j].Number })
	if isJSON(c) {
		return printJSON(versions)
	}
	printTable(versions, []string{"name", "number", "status", "files", "size", "error"})
	return nil
}

// liveAt is " at <address>" for a success message, or nothing when the address is unknown.
func liveAt(url string) string {
	if url == "" {
		return ""
	}
	return " at " + url
}

func scopeQuery(scope statico.Scope) string {
	values := url.Values{}
	if scope.WorkspaceID != "" {
		values.Set("workspaceId", scope.WorkspaceID)
	}
	if scope.ProjectID != "" {
		values.Set("projectId", scope.ProjectID)
	}
	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

// staticoSession authenticates and works out where to look for websites: the
// --workspace/--project given, else the project of futrou.json, else the whole
// workspace (the login default when none is given). The CLI's normal API token does every request, uploads
// included; Cloud authorises them and forwards the files to the node.
func staticoSession(c *cli.Context) (*services.ApiClient, statico.Scope, error) {
	client, err := requireAuth(c)
	if err != nil {
		return nil, statico.Scope{}, err
	}
	client.SetAfterMutation(nil) // one refresh per file would be thousands of requests
	var scope statico.Scope
	narrowed := c.String("workspace") != "" || c.String("project") != ""
	if !narrowed {
		if cfg, _, err := projectconfig.LoadConfig(".", ""); err == nil && cfg.Project != "" {
			narrowed = true
		}
	}
	// The API needs a workspace for every request: --workspace, futrou.json,
	// else the default chosen at login.
	if scope.WorkspaceID, err = resolveWorkspaceID(c); err != nil {
		return nil, scope, err
	}
	if narrowed {
		if scope.ProjectID, err = resolveProjectID(c, scope.WorkspaceID); err != nil {
			return nil, scope, err
		}
	}
	return client, scope, nil
}

// staticoName is the website a command is for: the first argument, else the one
// website futrou.json declares, else the name of the current folder.
func staticoName(c *cli.Context) (string, error) {
	if name := strings.TrimSpace(c.Args().First()); name != "" {
		return name, nil
	}
	return defaultStaticoName()
}

// defaultStaticoName is the one website futrou.json declares, else the name of the current folder.
func defaultStaticoName() (string, error) {
	if cfg, _, err := projectconfig.LoadConfig(".", ""); err == nil && len(cfg.Staticos) == 1 && cfg.Staticos[0].Name != "" {
		return cfg.Staticos[0].Name, nil
	}
	dir, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	if name := filepath.Base(dir); name != "" && name != "." && name != string(filepath.Separator) {
		return name, nil
	}
	return "", fmt.Errorf("statico website name required: this folder has no name to use")
}

// staticoTarget resolves the website a command is for (see staticoName).
func staticoTarget(c *cli.Context) (*services.ApiClient, *statico.Statico, error) {
	ref, err := staticoName(c)
	if err != nil {
		return nil, nil, err
	}
	return staticoTargetNamed(c, ref)
}

// staticoTargetNamed finds the website called ref.
func staticoTargetNamed(c *cli.Context, ref string) (*services.ApiClient, *statico.Statico, error) {
	client, scope, err := staticoSession(c)
	if err != nil {
		return nil, nil, err
	}
	st, err := statico.FindStatico(client, ref, scope)
	if err != nil {
		return nil, nil, err
	}
	return client, st, nil
}

// declaredStatico returns the website futrou.json declares under name, with the
// config it is in, or nil.
func declaredStatico(name string) (*projectconfig.StaticoConfig, *projectconfig.Config) {
	cfg, _, err := projectconfig.LoadConfig(".", "")
	if err != nil {
		return nil, nil
	}
	for i := range cfg.Staticos {
		if cfg.Staticos[i].Name == name {
			return &cfg.Staticos[i], cfg
		}
	}
	return nil, nil
}

// staticoRules reads the include and exclude flags as rules: the includes in
// order, then the excludes.
func staticoRules(c *cli.Context) (utils.GlobRules, error) {
	var rules utils.GlobRules
	specs := c.StringSlice("include")
	for _, spec := range specs {
		rule, err := utils.ParseGlobRule(spec)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	for _, source := range c.StringSlice("exclude") {
		if strings.TrimSpace(source) == "" {
			return nil, fmt.Errorf("--exclude needs a source")
		}
		rules = append(rules, utils.GlobRule{Source: source, Exclude: true})
	}
	return rules, nil
}

// errNothingIncluded is what a deploy says when it was told no file or folder to send.
var errNothingIncluded = fmt.Errorf("no files or folders were included to deploy: pass --include <file|folder|glob> or declare the website's include in futrou.json")

// hasIncluded reports whether the rules select anything (an --exclude alone does not).
func hasIncluded(rules utils.GlobRules) bool {
	for _, r := range rules {
		if !r.Exclude {
			return true
		}
	}
	return false
}

// staticoPlanFor is what upload and sync send: the declared website's own
// selection when no include is given, else the flags, relative to the current folder.
func staticoPlanFor(c *cli.Context, name string) (statico.Selection, string, statico.UploadOptions, error) {
	var opts statico.UploadOptions
	declared, cfg := declaredStatico(name)
	if declared != nil {
		answers, err := declared.VersionOptions()
		if err != nil {
			return statico.Selection{}, "", opts, err
		}
		opts.VersionOptions = answers
	}
	rules, err := staticoRules(c)
	if err != nil {
		return statico.Selection{}, "", opts, err
	}
	var sel statico.Selection
	base := "."
	switch {
	case len(c.StringSlice("include")) > 0 || declared == nil:
		sel = statico.Selection{Rules: rules}
	default:
		sel, base = cfg.StaticoSelection(*declared), cfg.BaseDir()
		sel.Rules = append(sel.Rules, rules...) // an --exclude refines the declared selection
	}
	if !c.Bool("empty") && !hasIncluded(sel.Rules) {
		return sel, base, opts, errNothingIncluded
	}
	if c.IsSet("html-to-folders") {
		sel.NoHTMLToFolders = !c.Bool("html-to-folders")
	}
	opts.Name = c.String("version")
	opts.Empty = c.Bool("empty")
	opts.NoActivate = c.IsSet("activate") && !c.Bool("activate")
	if c.IsSet("default-file") {
		opts.DefaultFile = c.String("default-file")
	}
	if c.IsSet("default-status") {
		opts.DefaultStatus = c.Int("default-status")
	}
	if c.IsSet("fallback-file") {
		opts.FallbackFile = c.String("fallback-file")
	}
	if c.IsSet("fallback-status") {
		opts.FallbackStatus = c.Int("fallback-status")
	}
	return sel, base, opts, nil
}

// createStatico creates the website an upload is for. A website declared in
// futrou.json brings its region and visibility; otherwise the flags do (public by
// default). The website goes into the workspace and project of the flags or the
// config, else the login's default.
func createStatico(c *cli.Context, client *services.ApiClient, name string) (*statico.Statico, error) {
	body := map[string]interface{}{"name": name, "public": c.Bool("public")}
	if declared, _ := declaredStatico(name); declared != nil {
		if declared.Public != nil && !c.IsSet("public") {
			body["public"] = *declared.Public
		}
		if declared.RegionId != "" && !c.IsSet("region") {
			body["regionId"] = declared.RegionId
		}
	}
	if c.String("region") != "" {
		body["region"] = c.String("region")
	}
	workspaceID, err := resolveWorkspaceID(c)
	if err != nil {
		return nil, err
	}
	projectID, err := resolveProjectID(c, workspaceID)
	if err != nil {
		return nil, err
	}
	body["workspaceId"], body["projectId"] = workspaceID, projectID
	var created statico.Statico
	status, err := client.RequestInto("POST", "/v2/staticos", body, &created)
	if err != nil {
		return nil, fmt.Errorf("creating the statico website %q: %w", name, err)
	}
	if status >= 400 || created.ID == "" {
		return nil, fmt.Errorf("creating the statico website %q: request failed with status %d", name, status)
	}
	return &created, nil
}

func runStaticoDeploy(c *cli.Context) error {
	// install (once) and build, before anything else happens
	if cfg, _, err := projectconfig.LoadConfig(".", ""); err == nil {
		if err := runProjectCommands(c, cfg); err != nil {
			return err
		}
	}
	name, err := staticoName(c)
	if err != nil {
		return err
	}
	client, scope, err := staticoSession(c)
	if err != nil {
		return err
	}
	defer cancelOnInterrupt(client)() // Ctrl+C stops the upload and marks the version failed
	// a website that does not exist yet is created by the first upload (a UUID can only be an existing one)
	st, err := statico.FindStatico(client, name, scope)
	var missing *statico.NotFoundError
	if errors.As(err, &missing) && !utils.IsUuid(name) {
		st, err = nil, nil
	}
	if err != nil {
		return err
	}
	if version := c.String("version"); version != "" && st != nil && len(c.StringSlice("include")) == 0 {
		exists, err := statico.VersionExists(client, *st, version)
		if err != nil {
			return err
		}
		if exists { // a version that exists is served, not uploaded again
			if err := statico.ActivateVersion(client, *st, version); err != nil {
				return err
			}
			if isJSON(c) {
				return printJSON(map[string]string{"version": version, "url": st.URL()})
			}
			fmt.Printf("✓ %s: version %s is live%s\n", st.Name, version, liveAt(st.URL()))
			return nil
		}
	}
	sel, base, opts, err := staticoPlanFor(c, name)
	if err != nil {
		return err
	}
	// .futrou holds the staged files of uploads; whatever an earlier run left there is cleared.
	if err := statico.ClearWorkDir(base); err != nil {
		return err
	}
	if !isJSON(c) {
		opts.Progress = utils.NewTransferProgress(os.Stdout, utils.IsTerminal(os.Stdout))
	}

	if c.Bool("dry-run") {
		diff, err := statico.Check(client, st, base, sel, opts)
		if err != nil {
			return err
		}
		if st == nil {
			fmt.Printf("The statico website %s does not exist yet: the upload would create it\n", name)
		}
		fmt.Println(diff.Describe())
		for _, f := range diff.Plan.Upload {
			fmt.Printf("  ↑ %s\n", f.Path)
		}
		for _, p := range diff.Plan.Delete {
			fmt.Printf("  ✗ %s\n", p)
		}
		if n := len(diff.Options.Redirects); n > 0 {
			fmt.Printf("  %d redirect(s)\n", n)
		}
		for _, w := range diff.Resolved.Warnings {
			fmt.Printf("  warning: %s\n", w)
		}
		if !diff.Changed() {
			fmt.Printf("%s is already up to date\n", name)
		}
		return nil
	}
	if st == nil {
		// find out the files are fine before a website is made for them
		if _, err := statico.Check(client, nil, base, sel, opts); err != nil {
			return err
		}
		if st, err = createStatico(c, client, name); err != nil {
			return err
		}
		if !isJSON(c) {
			fmt.Printf("✓ Created statico website %s\n", st.Name)
		}
	}
	result, err := statico.Sync(client, *st, base, sel, opts)
	if err != nil {
		return err
	}
	if isJSON(c) {
		return printJSON(result)
	}
	fmt.Println(result.Message(st.Name))
	return nil
}
