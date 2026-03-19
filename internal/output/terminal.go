package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/AtomicWasTaken/surge/internal/model"
	"github.com/charmbracelet/lipgloss"
)

// TerminalOutput renders review results to the terminal with colors.
type TerminalOutput struct {
	severityStyle map[model.Severity]lipgloss.Style
}

// NewTerminalOutput creates a new terminal output renderer.
func NewTerminalOutput() *TerminalOutput {
	return &TerminalOutput{
		severityStyle: map[model.Severity]lipgloss.Style{
			model.SeverityCritical: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true),
			model.SeverityHigh:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00")).Bold(true),
			model.SeverityMedium:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")),
			model.SeverityLow:      lipgloss.NewStyle().Foreground(lipgloss.Color("#1E90FF")),
			model.SeverityInfo:     lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")),
		},
	}
}

// Render prints the review result to stdout with colors.
func (t *TerminalOutput) Render(result *model.ReviewResult) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println()

	// Summary
	lines := wrapText(result.Summary, 60)
	for _, line := range lines {
		fmt.Println("  " + line)
	}
	fmt.Println()

	if len(result.Warnings) > 0 {
		fmt.Println("  Warnings")
		fmt.Println(strings.Repeat("─", 60))
		for _, warning := range result.Warnings {
			RenderWarning(warning)
		}
		fmt.Println()
	}

	// Findings
	if len(result.Findings) > 0 {
		fmt.Println("  Findings")
		fmt.Println(strings.Repeat("─", 60))

		bySeverity := groupBySeverity(result.Findings)
		for _, sev := range []model.Severity{model.SeverityCritical, model.SeverityHigh, model.SeverityMedium, model.SeverityLow, model.SeverityInfo} {
			findings := bySeverity[sev]
			if len(findings) == 0 {
				continue
			}

			style := t.severityStyle[sev]
			for _, f := range findings {
				// Severity badge
				badge := fmt.Sprintf("[%s]", strings.ToUpper(string(f.Severity)))
				if f.File != "" {
					if f.Line > 0 {
						fmt.Printf("  %s %s %s:%d\n", style.Render(badge), style.Render(f.Title), f.File, f.Line)
					} else {
						fmt.Printf("  %s %s %s\n", style.Render(badge), style.Render(f.Title), f.File)
					}
				} else {
					fmt.Printf("  %s %s\n", style.Render(badge), style.Render(f.Title))
				}

				// Body (indented)
				bodyLines := wrapText(f.Body, 56)
				for _, line := range bodyLines {
					fmt.Printf("    %s\n", line)
				}
				fmt.Println()
			}
		}
	}

	// Vibe Check
	fmt.Println("  Vibe Check")
	fmt.Println(strings.Repeat("─", 60))
	t.renderVibeCheck(result.VibeCheck)
	fmt.Println()

	// Stats
	fmt.Printf("  %d files reviewed  |  %d tokens in / %d tokens out  |  %.1fs\n",
		result.Stats.FilesReviewed, result.Stats.TokensIn, result.Stats.TokensOut, result.Stats.Duration)

	// Approval
	approveStr := "  ❌ Request Changes"
	if result.Approve {
		approveStr = "  ✅ Approve"
	}
	fmt.Println(approveStr)
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println()
}

func (t *TerminalOutput) renderVibeCheck(vc model.VibeCheck) {
	// Score bar
	score := vc.Score
	if score < 0 {
		score = 0
	}
	if score > 10 {
		score = 10
	}

	barLen := 20
	filled := (score * barLen) / 10

	var bar strings.Builder
	bar.WriteString("[")
	for i := 0; i < barLen; i++ {
		if i < filled {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}
	bar.WriteString("]")

	color := lipgloss.Color("#00FF00") // green
	if score < 5 {
		color = lipgloss.Color("#FF0000") // red
	} else if score < 7 {
		color = lipgloss.Color("#FFD700") // yellow
	}

	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	fmt.Printf("  %s %d/10 -- %s\n", style.Render(bar.String()), score, vc.Verdict)

	if len(vc.Flags) > 0 {
		fmt.Printf("  Flags: ")
		for i, flag := range vc.Flags {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("`%s`", flag)
		}
		fmt.Println()
	}
}

func wrapText(text string, width int) []string {
	var lines []string
	words := strings.Fields(text)
	var current strings.Builder

	for _, word := range words {
		if current.Len()+len(word)+1 > width {
			if current.Len() > 0 {
				lines = append(lines, current.String())
				current.Reset()
			}
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(word)
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	if len(lines) == 0 {
		lines = []string{""}
	}

	return lines
}

// RenderError prints an error message to stderr.
func RenderError(msg string) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	fmt.Fprintf(os.Stderr, "  %s %s\n", style.Render("ERROR:"), msg)
}

// RenderWarning prints a warning message to stdout.
func RenderWarning(msg string) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true)
	fmt.Printf("  %s %s\n", style.Render("WARNING:"), msg)
}
