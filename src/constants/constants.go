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

	DocsUrl = "https://futrou.com/docs"
	RepoUrl = "https://github.com/futrou/futrou-cli"
	McpUrl  = "https://mcp.futrou.com"
	LogoUrl = "https://raw.githubusercontent.com/futrou/futrou-cli/main/.github/assets/favicon.png"

	ApiUrl      = "https://api.futrou.com"
	DevApiUrl   = "https://api.dev.futrou.com"
	LocalApiUrl = "http://localhost:4000"

	UpgradeUnixUrl    = "https://futrou.com/install.sh"
	UpgradeWindowsUrl = "https://futrou.com/install.ps1"

	ProjectConfigSchemaURL = "https://futrou.com/futrou.schema.json"

	DefaultLogLevel  = "info"
	DefaultLogFormat = "plain"

	ConfigDir  = ".futrou"
	ConfigFile = "cli.json"
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
