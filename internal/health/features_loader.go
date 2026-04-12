package health

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// RegisterFeaturesFromDir walks dir for *.feature files, parses their name and
// tag metadata, and registers each one with the server as "unclaimed".
// Failures are logged and skipped — missing dir is not an error.
func RegisterFeaturesFromDir(s *Server, dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		slog.Debug("features dir not found, skipping feature registration", "dir", dir)
		return
	}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".feature") {
			return err
		}
		f, parseErr := parseFeatureFile(path)
		if parseErr != nil {
			slog.Warn("failed to parse feature file", "path", path, "error", parseErr)
			return nil
		}
		s.RegisterFeature(f)
		slog.Debug("registered feature", "id", f.ID, "name", f.Name)
		return nil
	})
}

// parseFeatureFile extracts Feature metadata from a single .feature file.
func parseFeatureFile(path string) (Feature, error) {
	f, err := os.Open(path)
	if err != nil {
		return Feature{}, err
	}
	defer func() { _ = f.Close() }()

	feat := Feature{
		FilePath: path,
		Status:   "unclaimed",
	}

	scanner := bufio.NewScanner(f)
	scenarios := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "@") {
			feat.Tags = append(feat.Tags, strings.Fields(line)...)
			continue
		}
		if strings.HasPrefix(line, "Feature:") {
			feat.Name = strings.TrimSpace(strings.TrimPrefix(line, "Feature:"))
			continue
		}
		if strings.HasPrefix(line, "Scenario:") || strings.HasPrefix(line, "Scenario Outline:") {
			scenarios++
		}
	}
	if err := scanner.Err(); err != nil {
		return Feature{}, err
	}

	feat.Scenarios = scenarios

	// Derive a stable ID from the file path relative to the features/ root.
	// e.g. "features/federation-mesh.feature" → "ocd/federation-mesh"
	rel := filepath.ToSlash(path)
	rel = strings.TrimPrefix(rel, "features/")
	rel = strings.TrimSuffix(rel, ".feature")
	// Strip a leading step_definitions/ segment if the walk dipped in
	rel = strings.TrimPrefix(rel, "step_definitions/")
	feat.ID = "ocd/" + rel

	if feat.Name == "" {
		feat.Name = strings.TrimSuffix(filepath.Base(path), ".feature")
	}

	return feat, nil
}
