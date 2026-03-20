package output

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalOutputRender(t *testing.T) {
	out := NewTerminalOutput()
	result := &model.ReviewResult{
		Summary:  "This summary should wrap onto multiple lines for the renderer to exercise the wrapping path.",
		Warnings: []string{"partial context"},
		Findings: []model.Finding{
			{
				Severity:   model.SeverityCritical,
				Title:      "Critical issue",
				File:       "a.go",
				Line:       10,
				Body:       "Detailed explanation for a critical issue in the change.",
				Suggestion: "Use safe output \x1b[31mwithout control bytes\non one line.",
			},
			{
				Severity: model.SeverityInfo,
				Title:    "Informational note",
				Body:     "No file attached.",
			},
		},
		VibeCheck: model.VibeCheck{
			Score:   8,
			Verdict: "Mostly fine",
			Flags:   []string{"generated"},
		},
		Stats: model.ReviewStats{
			FilesReviewed: 3,
			TokensIn:      100,
			TokensOut:     50,
			Duration:      2.5,
		},
		Approve: true,
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	out.Render(result)

	require.NoError(t, w.Close())
	os.Stdout = stdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	rendered := buf.String()
	assert.Contains(t, rendered, "Warnings")
	assert.Contains(t, rendered, "Findings")
	assert.Contains(t, rendered, "Critical issue")
	assert.Contains(t, rendered, "Fix: Use safe output")
	assert.Contains(t, rendered, "Vibe Check")
	assert.Contains(t, rendered, "Flags: `generated`")
	assert.Contains(t, rendered, "✅ Approve")
}

func TestRenderVibeCheckClampsScoreAndOmitsFlagsWhenEmpty(t *testing.T) {
	out := NewTerminalOutput()

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	out.renderVibeCheck(model.VibeCheck{Score: -3, Verdict: "Bad"})
	out.renderVibeCheck(model.VibeCheck{Score: 15, Verdict: "Great"})

	require.NoError(t, w.Close())
	os.Stdout = stdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	rendered := buf.String()
	assert.Contains(t, rendered, "0/10")
	assert.Contains(t, rendered, "10/10")
	assert.NotContains(t, rendered, "Flags:")
}

func TestWrapTextAndSanitizeHelpers(t *testing.T) {
	assert.Equal(t, []string{""}, wrapText("", 10))
	assert.Equal(t, []string{"one", "two"}, wrapText("one two", 4))
	assert.Equal(t, "hello world  next", sanitizeTerminalText("hello\x1b[31m world\t\nnext"))
}

func TestRenderErrorAndWarning(t *testing.T) {
	stdout := os.Stdout
	stderr := os.Stderr
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = outW
	os.Stderr = errW

	RenderWarning("careful")
	RenderError("broken")

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	os.Stdout = stdout
	os.Stderr = stderr

	var outBuf, errBuf bytes.Buffer
	_, err = io.Copy(&outBuf, outR)
	require.NoError(t, err)
	_, err = io.Copy(&errBuf, errR)
	require.NoError(t, err)
	require.NoError(t, outR.Close())
	require.NoError(t, errR.Close())

	assert.Contains(t, outBuf.String(), "WARNING:")
	assert.Contains(t, errBuf.String(), "ERROR:")
}

func TestTerminalRenderWithoutFindingsAndLowVibe(t *testing.T) {
	out := NewTerminalOutput()
	result := &model.ReviewResult{
		Summary: "Short summary.",
		VibeCheck: model.VibeCheck{
			Score:   3,
			Verdict: "Rough",
		},
		Stats:   model.ReviewStats{},
		Approve: false,
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	out.Render(result)
	require.NoError(t, w.Close())
	os.Stdout = stdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	rendered := buf.String()
	assert.NotContains(t, rendered, "Findings\n")
	assert.Contains(t, rendered, "❌ Request Changes")
}

func TestTerminalRenderFindingWithoutLine(t *testing.T) {
	out := NewTerminalOutput()
	result := &model.ReviewResult{
		Summary: "Summary.",
		Findings: []model.Finding{
			{Severity: model.SeverityLow, Title: "Low issue", File: "a.go", Body: "Body"},
		},
		VibeCheck: model.VibeCheck{Score: 6, Verdict: "Okay"},
		Approve:   true,
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	out.Render(result)
	require.NoError(t, w.Close())
	os.Stdout = stdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "Low issue a.go")
}

func TestRenderVibeCheckMultipleFlagsCommaBranch(t *testing.T) {
	out := NewTerminalOutput()
	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	out.renderVibeCheck(model.VibeCheck{Score: 6, Verdict: "Okay", Flags: []string{"a", "b"}})

	require.NoError(t, w.Close())
	os.Stdout = stdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "`a`, `b`")
}
