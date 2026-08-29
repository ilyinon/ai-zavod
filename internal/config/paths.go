package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	CodeDir     string `json:"codeDir"`
	ProjectsDir string `json:"projectsDir"`
	DBPath      string `json:"dbPath"`
}

func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	codeDir := filepath.Join(home, "dev_ai_zavod")
	return Paths{
		CodeDir:     codeDir,
		ProjectsDir: filepath.Join(home, "ai_zavod"),
		DBPath:      filepath.Join(codeDir, "zavod.db"),
	}, nil
}

func EnsureBaseDirs(paths Paths) error {
	for _, dir := range []string{paths.CodeDir, paths.ProjectsDir, filepath.Dir(paths.DBPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
