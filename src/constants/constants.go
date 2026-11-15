package constants

var (
	Name    = "Futrou CLI"
	Version = "0.0.0"
	Mode    = "development"

	Description = "Deploy and manage Futrou Cloud from your terminal."

	// ComposeFiles are the Docker Compose file names, in the order the Compose
	// spec prefers them.
	ComposeFiles = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

	// ConfigExtensions are the config file formats, in discovery order: JSON
	// first, then executable configuration, then YAML.
	ConfigExtensions = []string{"json", "js", "cjs", "mjs", "ts", "yaml", "yml"}

	// DefaultConfigNames are the config file base names used when no project is
	// selected.
	DefaultConfigNames = []string{"futrou"}

	// ProjectConfigFiles is the whole default discovery order: every format of
	// futrou, and last a Docker Compose file, which is converted to the
	// futrou.json equivalent.
	ProjectConfigFiles = configFileNames(DefaultConfigNames, ConfigExtensions, ComposeFiles)
)

const (
	EnvApiUrl    = "FUTROU_API_URL"
	EnvApiToken  = "FUTROU_API_TOKEN"
	EnvLogLevel  = "FUTROU_LOG_LEVEL"
	EnvLogFormat = "FUTROU_LOG_FORMAT"
	// EnvUpgradeDisabled makes `futrou upgrade` refuse and point to npm. The npm wrapper sets it.
	EnvUpgradeDisabled = "FUTROU_UPGRADE_DISABLED"

	DocsUrl = "https://futrou.com/docs"
	RepoUrl = "https://github.com/futrou/futrou-cli"
	McpUrl  = "https://mcp.futrou.com"
	LogoUrl = "https://raw.githubusercontent.com/futrou/futrou-cli/main/.github/assets/favicon.png"

	ApiUrl      = "https://api.futrou.com"
	DevApiUrl   = "https://api.dev.futrou.com"
	LocalApiUrl = "http://localhost:4000"

	UpgradeUnixUrl    = "https://raw.githubusercontent.com/futrou/futrou-cli/main/install.sh"
	UpgradeWindowsUrl = "https://raw.githubusercontent.com/futrou/futrou-cli/main/install.ps1"

	ProjectConfigSchemaURL = "https://futrou.com/futrou.schema.json"

	DefaultLogLevel  = "info"
	DefaultLogFormat = "text"

	// CliConfigDir is the CLI's own folder in the user's home directory (~/.futrou),
	// which holds CliConfigFile.
	CliConfigDir  = ".futrou"
	CliConfigFile = "cli.json"

	// The folders a project run keeps files in, relative to the folder of the
	// project's config file. BuildDir is inside the project (not the CliConfigDir of the
	// home directory) and holds what a run stages and caches, in the folders below it.
	BuildDir   = ".futrou"
	CacheDir   = BuildDir + "/cache"   // survives from run to run: hashes, finished installs
	StaticoDir = BuildDir + "/statico" // staged files of a statico upload, <name>/<version>
	TmpDir     = BuildDir + "/tmp"     // scratch files of a run, cleared at the start of the next
	DebugDir   = BuildDir + "/debug"   // what a run writes for debugging (requests, logs), cleared at the start of the next
)

// configFileNames lists every base.extension combination (base-major), then the
// extra names.
func configFileNames(bases, extensions, extra []string) []string {
	var names []string
	for _, base := range bases {
		for _, ext := range extensions {
			names = append(names, base+"."+ext)
		}
	}
	return append(names, extra...)
}

// ProjectConfigFilesFor lists the config files that select the named project:
// futrou.<project>.json, futrou.<project>.js, ...
func ProjectConfigFilesFor(project string) []string {
	return configFileNames([]string{"futrou." + project}, ConfigExtensions, nil)
}
