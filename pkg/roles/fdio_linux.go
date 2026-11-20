//go:build linux

package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func openRegularInside(root, path string) (*os.File, error) {
	label := filepath.Base(path)
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", label, err)
	}
	resolved, err := fdPath(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("%s: %w", label, ErrEscape)
	}
	inside, err := pathInsideRoot(root, resolved)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("resolve %s: %w", label, err)
	}
	if !inside {
		file.Close()
		return nil, fmt.Errorf("%s: %w", label, ErrEscape)
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
	label := filepath.Base(path)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, fmt.Errorf("open departments directory: %w", err)
	}
	resolved, err := fdPath(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("%s: %w", label, ErrEscape)
	}
	inside, err := pathInsideRoot(root, resolved)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("resolve %s: %w", label, err)
	}
	if !inside {
		file.Close()
		return nil, fmt.Errorf("%s: %w", label, ErrEscape)
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

func fdPath(file *os.File) (string, error) {
	link := fmt.Sprintf("/proc/self/fd/%d", file.Fd())
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		return "", fmt.Errorf("relative fd path")
	}
	return filepath.EvalSymlinks(target)
}

func pathInsideRoot(root, candidate string) (bool, error) {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}
