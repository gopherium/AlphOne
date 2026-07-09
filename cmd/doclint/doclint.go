// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

// violation records a function declaration that lacks a doc comment.
type violation struct {
	file string
	line int
	name string
}

// collectViolations walks the Go source under root and reports every
// function and method that has no doc comment, skipping test files and
// generated files.
func collectViolations(root string) ([]violation, error) {
	var violations []violation
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		found, err := violationsInFile(path)
		if err != nil {
			return err
		}
		violations = append(violations, found...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("doclint: %w", err)
	}
	return violations, nil
}

// violationsInFile parses one Go file and returns its undocumented
// functions, or nothing if the file is generated.
func violationsInFile(path string) ([]violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("doclint: parsing %s: %w", path, err)
	}
	if isGenerated(file) {
		return nil, nil
	}
	var violations []violation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc != nil {
			continue
		}
		position := fset.Position(fn.Pos())
		violations = append(violations, violation{file: path, line: position.Line, name: fn.Name.Name})
	}
	return violations, nil
}

// isGenerated reports whether file carries the standard generated-code
// marker.
func isGenerated(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.Contains(comment.Text, "Code generated") && strings.Contains(comment.Text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}

// run reports every documentation violation under root to out and fails
// when any exist.
func run(root string, out io.Writer) error {
	violations, err := collectViolations(root)
	if err != nil {
		return err
	}
	for _, v := range violations {
		_, _ = fmt.Fprintf(out, "%s:%d: %s is missing a doc comment\n", v.file, v.line, v.name)
	}
	if len(violations) > 0 {
		return fmt.Errorf("doclint: %d undocumented functions", len(violations))
	}
	return nil
}
