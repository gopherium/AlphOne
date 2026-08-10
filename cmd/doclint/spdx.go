// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// licensedExtensions lists the source extensions that must carry a license header.
var licensedExtensions = map[string]bool{
	".go":  true,
	".ts":  true,
	".tsx": true,
	".js":  true,
}

// skippedDirs lists the directories holding files no license header is expected in.
var skippedDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".astro":       true,
	"dist":         true,
	"coverage":     true,
	"gql":          true,
	"docs":         true,
}

// headerScanLines bounds how far into a file the header is looked for.
const headerScanLines = 5

// collectUnlicensed walks the source under root and reports every file
// missing an SPDX header, skipping generated files.
func collectUnlicensed(root string) ([]string, error) {
	var unlicensed []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skippedDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !licensedExtensions[filepath.Ext(path)] {
			return nil
		}
		licensed, err := carriesLicense(path)
		if err != nil {
			return err
		}
		if !licensed {
			unlicensed = append(unlicensed, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("doclint: %w", err)
	}
	return unlicensed, nil
}

// carriesLicense reports whether the file opens with an SPDX header or is generated.
func carriesLicense(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("doclint: opening %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	return scanHeader(file, path)
}

// scanHeader reports whether the opening lines of r carry a header.
func scanHeader(r io.Reader, path string) (bool, error) {
	scanner := bufio.NewScanner(r)
	for line := 0; line < headerScanLines && scanner.Scan(); line++ {
		if isHeaderLine(scanner.Text()) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("doclint: reading %s: %w", path, err)
	}
	return false, nil
}

// isHeaderLine reports whether the line licenses the file or marks it generated.
func isHeaderLine(text string) bool {
	if strings.Contains(text, "SPDX-License-Identifier") {
		return true
	}
	return strings.Contains(text, "Code generated") && strings.Contains(text, "DO NOT EDIT")
}
