package model

import "time"

// PR represents a pull request.
type PR struct {
	Number       int
	Title        string
	Body         string
	State        string
	Author       string
	BaseRef      string
	HeadRef      string
	BaseSHA      string
	HeadSHA      string
	ChangedFiles int
	Additions    int
	Deletions    int
	URL          string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// FileChange represents a file that was changed in a PR.
type FileChange struct {
	Path      string
	Status    FileStatus // added, modified, deleted, renamed
	Additions int
	Deletions int
	Patch     string // unified diff patch
}

// FileStatus represents the type of file change.
type FileStatus string

const (
	FileStatusAdded    FileStatus = "added"
	FileStatusModified FileStatus = "modified"
	FileStatusDeleted  FileStatus = "deleted"
	FileStatusRenamed  FileStatus = "renamed"
)

// Diff represents the full diff of a PR.
type Diff struct {
	Files []FileChange
}

// Hunk represents a contiguous block of changes in a diff.
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []DiffLine
}

// DiffLine represents a single line in a diff hunk.
type DiffLine struct {
	Type     DiffLineType
	Content  string
	OldLine  int
	NewLine  int
	Position int // position in the patch (for GitHub API)
}

// DiffLineType represents the type of diff line.
type DiffLineType string

const (
	DiffLineContext DiffLineType = "context"
	DiffLineAdd     DiffLineType = "add"
	DiffLineDelete  DiffLineType = "delete"
)
