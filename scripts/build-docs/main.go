// build-docs regenerates the CLI command reference embedded in SKILL.md and
// README.md from the command definitions.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"futrou-cli/src/commands"
)

var docFiles = []string{"SKILL.md", "README.md"}

func main() {
	reference := commands.ReferenceMarkdown()
	for _, file := range docFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			fatalf("reading %s: %v", file, err)
		}
		updated, err := commands.ReplaceReference(string(data), reference)
		if err != nil {
			fatalf("%s: %v", file, err)
		}
		for _, example := range commands.ExampleFiles {
			content, err := os.ReadFile(filepath.Join("examples", example.Name))
			if err != nil {
				fatalf("reading example %s: %v", example.Name, err)
			}
			updated = commands.ReplaceExample(updated, example.Name, example.Language, string(content))
		}
		if updated == string(data) {
			continue
		}
		if err := os.WriteFile(file, []byte(updated), 0o644); err != nil {
			fatalf("writing %s: %v", file, err)
		}
		fmt.Printf("  + %s\n", file)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "build-docs: "+format+"\n", args...)
	os.Exit(1)
}
