//go:build !linux

package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveInside(root, path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", filepath.Base(path), err)
	}
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", filepath.Base(path), err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: %w", filepath.Base(path), ErrEscape)
	}
	return real, nil
}

func openRegularInside(root, path string) (*os.File, error) {
	real, err := resolveInside(root, path)
	if err != nil {
		return nil, err
	}
	label := filepath.Base(path)
	file, err := os.Open(real)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", label, err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("%s is not a regular file", label)
	}
	return file, nil
}

func openDirInside(root, path string) (*os.File, error) {
	real, err := resolveInside(root, path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(real)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("open departments directory: %w", err)
	}
	if !info.IsDir() {
		file.Close()
		return nil, fmt.Errorf("open departments directory: %s is not a directory", path)
	}
	return file, nil
}
