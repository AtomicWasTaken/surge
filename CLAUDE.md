# surge - Claude Code Context

## Project Overview

**surge** is an AI-powered PR code review CLI tool written in Go. It analyzes pull request diffs and posts structured reviews with security findings, performance issues, logic bugs, code quality concerns, and a distinctive "vibe codability" check.

- **Repo**: github.com/AtomicWasTaken/surge
- **Go version**: 1.26.1
- **Main package**: `cmd/surge/main.go`

## Versioning

surge uses semantic versioning (MAJOR.MINOR.PATCH). Version info is injected at build time via git tags and ldflags:

```bash
# Version format: v1.2.3
# Injected via: -X main.version={{ .Version }} -X main.commit={{ .Commit }} -X main.date={{ .Date }}

git tag v0.1.0
git push origin v0.1.0  # Triggers release workflow automatically
```

The `release.yml` workflow builds and publishes binaries on every tag push.

## Project Structure

```
cmd/surge/main.go          # Entry point. Calls cli.Execute()
internal/
  cli/                     # Cobra CLI commands and flags
    root.go               # Root command, all subcommands, flag definitions
    review.go             # runReview - wires config, AI client, GitHub client, orchestrator
  config/
    config.go             # Config loading via Viper (YAML + env + flags)
  diff/
    parser.go             # Unified diff parser → FileChange structs
  github/
    client.go             # PRClient interface (enables future GitLab, etc.)
    github_client.go      # GitHub REST implementation
  ai/
    types.go              # AIClient interface, CompletionRequest/Response
    litellm.go           # OpenAI-compatible litellm proxy client
    claude.go            # Direct Anthropic Claude API client
  review/
    prompts.go           # System prompt + user prompt builder
    orchestrator.go      # Main pipeline: fetch diff → AI → parse → post
    output.go            # JSON parsing from AI response
    vibe.go              # Vibe codability heuristic detector
  output/
    markdown.go         # GFM markdown for GitHub PR comments
    terminal.go         # Colored terminal output with lipglight
    json.go             # Structured JSON output
  model/
    pr.go               # PR, FileChange, Hunk, DiffLine types
    review.go           # ReviewResult, Finding, Severity, VibeCheck types
pkg/
  httpclient/            # Shared HTTP client (currently unused, prefer stdlib http)
scripts/
  build.sh              # Cross-platform build script
  install.sh            # Installation script
```

## Build and Test

```bash
# Build
go build -o surge ./cmd/surge

# Test
go test ./...

# Lint
golangci-lint run ./...

# Cross-platform build
./scripts/build.sh

# Install locally
go install ./cmd/surge
```

## Release Process

1. Update version in code if needed (version is auto-injected from git tags)
2. Run tests: `go test ./...`
3. Run linter: `golangci-lint run ./...`
4. Tag and push:
   ```bash
   git tag v0.2.0
   git push origin v0.2.0
   ```
5. The `release.yml` workflow automatically builds binaries and creates a GitHub Release

## Key Design Decisions

### Config Precedence (handled by Viper)
CLI flags → Environment variables → Config file (surge.yaml) → Defaults

### AI Response Format
surge uses **structured JSON output** from the AI, not unstructured markdown parsing. The system prompt instructs the AI to respond with a specific JSON schema that maps directly to Go structs. This is more reliable than parsing natural language.

### Idempotent Reviews
Before posting a new review, surge deletes old comments with `<!-- SURGE -->` markers. Running surge twice replaces the previous review rather than piling on comments.

### Context Depth
- `diff-only` (default): Only the diff/patch is sent to the AI
- `relevant`: Full file content for changed files (truncated at 5000 lines)
- `full`: Full content plus dependency files

### Vibe Codability
The standout feature. Detects AI-generated code patterns:
- Generic boilerplate (try-catch wrappers with no specific handling)
- Over-engineering (interfaces for single implementations)
- Context blindness (ignoring existing codebase patterns)
- Wrong approach (idiomatically wrong for the language/framework)
- Confused about context (references non-existent files/functions)

Implemented via both AI instruction in the system prompt and post-processing heuristics in `vibe.go`.

## Important Patterns

### Adding a New AI Provider
1. Create `internal/ai/<provider>.go` implementing `AIClient` interface
2. Add provider name to `config.go` Validate()
3. Add case in `review.go` runReview() switch statement

### Adding a New PR Platform (e.g., GitLab)
1. Create `internal/gitlab/client.go` implementing `PRClient` interface
2. Add `--provider` flag to CLI
3. Wire it up in the orchestrator

### Prompt Iteration
Prompt changes are the highest-leverage way to improve review quality. Edit `prompts.go`:
- System prompt → `SystemPrompt()` - instructions, rules, output format
- User prompt → `BuildUserPrompt()` - dynamic content based on PR context

## Testing

- **Unit tests**: Config loading, diff parsing, vibe detection, JSON parsing, prompts
- **Integration tests**: Mock HTTP servers with recorded responses (not yet written)
- Tests live alongside implementation files as `*_test.go`

## CI/CD

- **CI** (`.github/workflows/ci.yml`): Runs tests and lints on every push/PR
- **Release** (`.github/workflows/release.yml`): Builds binaries on git tags
- **AI Review** (`.github/workflows/review.yml`): Runs surge on PRs (requires `SURGE_AI_API_KEY` secret)

## Common Issues

### "Failed to parse AI response as JSON"
The AI returned invalid JSON. Common causes:
- Model produced a non-JSON response (try adjusting temperature)
- The response was truncated (increase `maxTokens`)
- Check the raw response in verbose mode: `surge review --pr N --dry-run -v`

### "Review position is stale"
The diff lines have moved since surge fetched them. This happens when the PR changes between the files fetch and the review post. The solution is to run surge again after the PR stabilizes.

### Inline comments not appearing
GitHub inline comments use **patch positions** (line numbers in the unified diff), not file line numbers. The `findPositionInPatch()` function does approximate matching. If the diff has complex changes, positions may be stale.
