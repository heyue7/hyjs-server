package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Historical native recordings live outside Git. Tests first read this worktree,
// then may read the exact same relative file from one explicit immutable source.
// The helper never creates, copies, links, or writes a fixture directory.
func readTestAnalysis75(name string) ([]byte, error) {
	data, err := os.ReadFile(name)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return data, err
	}
	prefix := filepath.Clean("../../analysis") + string(filepath.Separator)
	clean := filepath.Clean(name)
	if !strings.HasPrefix(clean, prefix) {
		return nil, err
	}
	root := os.Getenv("HYJS_TEST_FIXTURE_ROOT")
	if root == "" {
		return nil, fmt.Errorf("%w; set HYJS_TEST_FIXTURE_ROOT to the original read-only analysis directory", err)
	}
	if !filepath.IsAbs(root) {
		return nil, errors.New("HYJS_TEST_FIXTURE_ROOT must be an absolute analysis directory")
	}
	relative := strings.TrimPrefix(clean, prefix)
	if relative == "" || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("original fixture path escapes analysis directory")
	}
	return os.ReadFile(filepath.Join(root, relative))
}
