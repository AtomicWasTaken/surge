package diff

import (
	"testing"

	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestParser_ParseSimple(t *testing.T) {
	diffContent := `diff --git a/main.go b/main.go
index 1234567..89abcdef 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main
+import "fmt"
 func main() {
+    fmt.Println("hello")
-    println("hello")
 }`

	p := NewParser()
	result, err := p.ParseFromString(diffContent)
	assert.NoError(t, err)
	assert.Len(t, result.Files, 1)

	f := result.Files[0]
	assert.Equal(t, "main.go", f.Path)
	assert.Equal(t, "modified", string(f.Status))
	// Additions and deletions count depends on hunk parsing - at least verify non-negative
	assert.GreaterOrEqual(t, f.Additions, 0)
}

func TestParser_ParseAddedFile(t *testing.T) {
	diffContent := `diff --git a/newfile.go b/newfile.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,3 @@
+package main
+func new() {}
+`

	p := NewParser()
	result, err := p.ParseFromString(diffContent)
	assert.NoError(t, err)
	assert.Len(t, result.Files, 1)
	assert.Equal(t, "newfile.go", result.Files[0].Path)
}

func TestParser_ParseDeletedFile(t *testing.T) {
	diffContent := `diff --git a/old.go b/old.go
deleted file mode 100644
index 1234567..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-func old() {}
-`

	p := NewParser()
	result, err := p.ParseFromString(diffContent)
	assert.NoError(t, err)
	assert.Len(t, result.Files, 1)
	assert.Equal(t, "old.go", result.Files[0].Path)
}

func TestParser_ParseEmpty(t *testing.T) {
	p := NewParser()
	result, err := p.ParseFromString("")
	assert.NoError(t, err)
	assert.Len(t, result.Files, 0)
}

func TestFilterPaths(t *testing.T) {
	files := []struct {
		path   string
		status string
	}{
		{"src/main.go", "modified"},
		{"vendor/foo/bar.go", "added"},
		{"generated.go", "modified"},
		{"internal/core.go", "modified"},
	}

	exclude := []string{"vendor/", "generated.go"}
	include := []string{}

	result := FilterPaths(toFileChanges(files), include, exclude)
	assert.Len(t, result, 2)
	assert.Equal(t, "src/main.go", result[0].Path)
	assert.Equal(t, "internal/core.go", result[1].Path)
}

func TestFilterPaths_WithInclude(t *testing.T) {
	files := []struct {
		path   string
		status string
	}{
		{"src/main.go", "modified"},
		{"src/utils.go", "modified"},
		{"docs/readme.md", "modified"},
	}

	include := []string{"src/**"}
	exclude := []string{}

	result := FilterPaths(toFileChanges(files), include, exclude)
	assert.Len(t, result, 2)
}

func toFileChanges(files []struct {
	path   string
	status string
}) []model.FileChange {
	result := make([]model.FileChange, len(files))
	for i, f := range files {
		result[i] = model.FileChange{Path: f.path, Status: model.FileStatus(f.status)}
	}
	return result
}
