package diff

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/AtomicWasTaken/surge/internal/model"
)

// Parser parses unified diff format into structured FileChange objects.
type Parser struct{}

// NewParser creates a new diff parser.
func NewParser() *Parser {
	return &Parser{}
}

// Parse reads a unified diff and returns a list of file changes.
func (p *Parser) Parse(r io.Reader) (*model.Diff, error) {
	scanner := bufio.NewScanner(r)
	var files []model.FileChange
	var currentFile *model.FileChange
	var currentHunk *model.Hunk
	position := 0 // position within the patch (for GitHub API)

	for scanner.Scan() {
		line := scanner.Text()
		position++

		// Detect new file in diff
		if strings.HasPrefix(line, "diff --git") {
			// Save previous file if any
			if currentFile != nil {
				files = append(files, *currentFile)
			}
			// Extract file path from "diff --git a/path b/path"
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				currentFile = &model.FileChange{
					Path: strings.TrimPrefix(parts[2], "a/"),
				}
			} else {
				currentFile = &model.FileChange{}
			}
			currentHunk = nil
			position = 0
			continue
		}

		// Detect new hunk
		hunkRe := regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)`)
		if matches := hunkRe.FindStringSubmatch(line); matches != nil {
			oldStart, _ := strconv.Atoi(matches[1])
			oldLines := 1
			if matches[2] != "" {
				oldLines, _ = strconv.Atoi(matches[2])
			}
			newStart, _ := strconv.Atoi(matches[1])
			if matches[3] != "" {
				newStart, _ = strconv.Atoi(matches[3])
			}
			newLines := 1
			if matches[4] != "" {
				newLines, _ = strconv.Atoi(matches[4])
			}
			position++
			currentHunk = &model.Hunk{
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
				Lines:    []model.DiffLine{},
			}
			continue
		}

		// Detect file status in the index line
		if strings.HasPrefix(line, "index ") {
			// index 1234567..89abcdef
			continue
		}

		// Detect file mode / rename
		if strings.HasPrefix(line, "new file") || strings.HasPrefix(line, "deleted file") ||
			strings.HasPrefix(line, "old mode") || strings.HasPrefix(line, "new mode") ||
			strings.HasPrefix(line, "similarity index") || strings.HasPrefix(line, "rename from") ||
			strings.HasPrefix(line, "rename to") {
			if currentFile != nil {
				if strings.HasPrefix(line, "new file") {
					currentFile.Status = model.FileStatusAdded
				} else if strings.HasPrefix(line, "deleted file") {
					currentFile.Status = model.FileStatusDeleted
				}
			}
			continue
		}

		// Detect --- and +++ lines (file headers in unified diff)
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}

		// Process diff content lines
		if currentFile != nil && currentHunk != nil {
			var diffLine model.DiffLine
			switch {
			case strings.HasPrefix(line, "+"):
				diffLine = model.DiffLine{Type: model.DiffLineAdd, Content: strings.TrimPrefix(line, "+"), Position: position}
				currentFile.Additions++
				currentHunk.NewLines++
			case strings.HasPrefix(line, "-"):
				diffLine = model.DiffLine{Type: model.DiffLineDelete, Content: strings.TrimPrefix(line, "-"), Position: position}
				currentFile.Deletions++
				currentHunk.OldLines++
			case strings.HasPrefix(line, " "):
				diffLine = model.DiffLine{Type: model.DiffLineContext, Content: strings.TrimPrefix(line, " "), Position: position}
			default:
				continue
			}
			currentHunk.Lines = append(currentHunk.Lines, diffLine)
		}
	}

	// Save last file
	if currentFile != nil {
		if currentFile.Status == "" {
			currentFile.Status = model.FileStatusModified
		}
		files = append(files, *currentFile)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading diff: %w", err)
	}

	return &model.Diff{Files: files}, nil
}

// ParseFromString parses a diff from a string.
func (p *Parser) ParseFromString(s string) (*model.Diff, error) {
	return p.Parse(strings.NewReader(s))
}

// FilterPaths filters file changes by include/exclude glob patterns.
func FilterPaths(files []model.FileChange, include, exclude []string) []model.FileChange {
	var result []model.FileChange
	for _, f := range files {
		if len(include) > 0 && !matchesAny(f.Path, include) {
			continue
		}
		if matchesAny(f.Path, exclude) {
			continue
		}
		result = append(result, f)
	}
	return result
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		// Simple glob matching - just check if pattern is a substring or has wildcards
		if strings.Contains(p, "*") {
			// Convert simple glob to regex-like matching
			regex := strings.ReplaceAll(p, ".", "\\.")
			regex = strings.ReplaceAll(regex, "**", ".*")
			regex = strings.ReplaceAll(regex, "*", "[^/]*")
			matched, _ := regexp.MatchString("^"+regex+"$", path)
			if matched {
				return true
			}
		} else if strings.HasPrefix(path, p) || path == p {
			return true
		}
	}
	return false
}
