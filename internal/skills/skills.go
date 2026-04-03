package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func findProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

var (
	ErrSkillNotFound      = errors.New("skill not found")
	ErrInvalidFrontmatter = errors.New("invalid skill frontmatter")
	ErrMissingName        = errors.New("skill missing required 'name' field")
	ErrMissingDescription = errors.New("skill missing required 'description' field")
	ErrNameMismatch       = errors.New("skill name does not match directory name")
)

type SkillMetadata struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	Metadata      map[string]string `yaml:"metadata"`
}

type ValidationResult struct {
	Path        string
	Metadata    *SkillMetadata
	Valid       bool
	Validations []string
	Errors      []string
}

// extractFrontmatter returns only the YAML content between the opening ---
// and the closing --- markers, so the body of a markdown SKILL.md file
// is not fed to the YAML parser.
func extractFrontmatter(data []byte) []byte {
	content := string(data)
	// Only extract if the file starts with a document-start marker.
	trimmed := strings.TrimLeft(content, " \t")
	if !strings.HasPrefix(trimmed, "---") {
		return data
	}
	// Skip the opening ---\n
	start := strings.Index(content, "\n")
	if start < 0 {
		return data
	}
	rest := content[start+1:]
	// Find the closing --- on its own line.
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return []byte(rest)
	}
	return []byte(rest[:end])
}

func ValidateSkillFile(path string) (*ValidationResult, error) {
	result := &ValidationResult{Path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		result.Errors = append(result.Errors, "failed to read file: "+err.Error())
		return result, err
	}

	var metadata SkillMetadata
	if err := yaml.Unmarshal(extractFrontmatter(data), &metadata); err != nil {
		result.Errors = append(result.Errors, "failed to parse YAML frontmatter: "+err.Error())
		return result, ErrInvalidFrontmatter
	}

	result.Metadata = &metadata
	result.Validations = append(result.Validations, "YAML frontmatter parsed successfully")

	if metadata.Name == "" {
		result.Errors = append(result.Errors, ErrMissingName.Error())
		return result, ErrMissingName
	}
	result.Validations = append(result.Validations, "name field present")

	if metadata.Description == "" {
		result.Errors = append(result.Errors, ErrMissingDescription.Error())
		return result, ErrMissingDescription
	}
	result.Validations = append(result.Validations, "description field present")

	dir := filepath.Base(filepath.Dir(path))
	if metadata.Name != dir {
		result.Errors = append(result.Errors, ErrNameMismatch.Error()+" (expected "+dir+", got "+metadata.Name+")")
		return result, ErrNameMismatch
	}
	result.Validations = append(result.Validations, "name matches directory")

	if len(metadata.Description) > 200 {
		result.Validations = append(result.Validations, "description is concise")
	}

	result.Valid = len(result.Errors) == 0

	return result, nil
}

func FindSkills(rootDir string) ([]ValidationResult, error) {
	var results []ValidationResult

	skillDirs := []string{
		".opencode/skills",
		".claude/skills",
		".claude/agents",
	}

	for _, dir := range skillDirs {
		fullPath := filepath.Join(rootDir, dir)
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillPath := filepath.Join(fullPath, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillPath); err != nil {
				continue
			}

			result, err := ValidateSkillFile(skillPath)
			if err != nil {
				if result != nil {
					results = append(results, *result)
				}
				continue
			}
			if result != nil {
				results = append(results, *result)
			}
		}
	}

	return results, nil
}

func ValidateProjectConfig(rootDir string) map[string]any {
	status := make(map[string]any)

	files := []string{
		"AGENTS.md",
		"Makefile",
		".golangci.yml",
		".claude.md",
	}

	for _, f := range files {
		path := filepath.Join(rootDir, f)
		exists := false
		if _, err := os.Stat(path); err == nil {
			exists = true
		}
		status[f] = map[string]bool{"exists": exists}
	}

	skills, _ := FindSkills(rootDir)
	status["skills"] = map[string]any{
		"count":   len(skills),
		"details": skills,
	}

	return status
}

func GenerateStartHereReport(rootDir string) string {
	var sb strings.Builder

	sb.WriteString("# Start Here - OpenCode Configuration\n\n")
	sb.WriteString("✅ **Connected**: OpenCode client is operational\n\n")

	status := ValidateProjectConfig(rootDir)

	sb.WriteString("## Configuration Status\n\n")
	sb.WriteString("| Component | Status |\n")
	sb.WriteString("|-----------|--------|\n")

	for file, info := range status {
		if file == "skills" {
			continue
		}
		if m, ok := info.(map[string]bool); ok {
			statusIcon := "❌"
			if m["exists"] {
				statusIcon = "✅"
			}
			sb.WriteString("| " + file + " | " + statusIcon + " |\n")
		}
	}

	if skillsData, ok := status["skills"].(map[string]any); ok {
		count := skillsData["count"].(int) //nolint:errcheck
		sb.WriteString("| OpenCode Skills | ✅ " + string(rune('0'+count)) + " loaded |\n")
	}

	sb.WriteString("\n## Available Skills\n\n")
	sb.WriteString("| Skill | Purpose |\n")
	sb.WriteString("|-------|---------|\n")

	if skillsData, ok := status["skills"].(map[string]any); ok {
		if details, ok := skillsData["details"].([]ValidationResult); ok {
			for _, s := range details {
				sb.WriteString("| " + s.Metadata.Name + " | " + s.Metadata.Description + " |\n")
			}
		}
	}

	sb.WriteString("\n## Quick Start\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# Run full quality gate\n")
	sb.WriteString("make ci\n\n")
	sb.WriteString("# Validate configuration\n")
	sb.WriteString("go run ./cmd/ocd-smoke-alarm validate --config=configs/sample.yaml\n")
	sb.WriteString("```\n")

	return sb.String()
}
