# surge

AI-powered PR code review with style.

`surge` analyzes your pull request diffs and provides structured, actionable feedback on security vulnerabilities, performance issues, logic bugs, code quality, and something special we call **vibe codability** -- detecting AI-generated code that is technically correct but soulless.

## Features

- **Rich PR summaries** with collapsible sections, severity-coded findings, and a distinctive visual presentation
- **Inline diff comments** so reviewers can see exactly where issues are
- **Vibe codability check** -- detects generic AI patterns, over-engineering, context blindness, and confused outputs
- **Multiple AI backends** -- use your own litellm proxy or direct Claude API
- **Configurable** -- YAML config file with smart defaults, everything overrideable via CLI flags or environment variables
- **Idempotent** -- re-running replaces the previous review, no comment spam
- **Universal CLI** -- drop into any CI system (GitHub Actions, GitLab CI, CircleCI, etc.)

## Installation

### Go install (recommended)

```bash
go install github.com/AtomicWasTaken/surge/cmd/surge@latest
```

### From source

```bash
git clone https://github.com/AtomicWasTaken/surge.git
cd surge
go install ./cmd/surge
```

## Quick Start

1. Create a `surge.yaml` in your repository root:

```yaml
ai:
  provider: litellm
  model: claude-sonnet-4-6
  baseUrl: http://localhost:4000
  apiKey: "${LITELLM_API_KEY}"
```

2. Set your environment variables:

```bash
export SURGE_GITHUB_TOKEN=ghp_your_github_token
export LITELLM_API_KEY=your_litellm_api_key
```

3. Run a review:

```bash
surge review --pr 123
```

## Configuration

`surge.yaml` supports the following options:

```yaml
ai:
  provider: litellm  # or "claude"
  model: claude-sonnet-4-6
  baseUrl: http://localhost:4000  # litellm proxy URL
  apiKey: "${LITELLM_API_KEY}"

contextDepth: diff-only  # diff-only | relevant | full

output:
  format: terminal  # terminal | markdown | json
  showStats: true

categories:
  security: true
  performance: true
  logic: true
  maintainability: true
  vibe: true

maxInlineComments: 20
maxTokens: 8192
temperature: 0.3

excludePaths:
  - "*.generated.go"
  - "vendor/**"
```

### Environment Variables

All config values can be set via environment variables:

| Variable | Description |
|----------|-------------|
| `SURGE_GITHUB_TOKEN` | GitHub personal access token |
| `SURGE_AI_API_KEY` | AI API key |
| `SURGE_AI_MODEL` | AI model name |
| `SURGE_AI_PROVIDER` | `litellm` or `claude` |
| `SURGE_AI_BASE_URL` | litellm proxy URL |
| `SURGE_CONTEXT_DEPTH` | `diff-only`, `relevant`, or `full` |
| `SURGE_OUTPUT` | `terminal`, `markdown`, or `json` |
| `SURGE_DRY_RUN` | Print without posting |

## GitHub Actions

Add to your workflow:

```yaml
name: AI Code Review
on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install surge
        run: curl -sSL https://install.surge.sh | sh

      - name: Run review
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          SURGE_AI_API_KEY: ${{ secrets.SURGE_AI_API_KEY }}
        run: surge review --pr ${{ github.event.pull_request.number }}
```

## CLI Reference

```bash
surge review [flags]           Run a code review
surge config init              Create a surge.yaml config file
surge config validate          Validate the config file
surge diff --pr N              Show diff for a PR (no review)

Flags:
  --pr N                    PR number
  --owner, --repo           Repository owner and name (auto-detected)
  --dry-run                 Print without posting to PR
  --config FILE             Config file path
  --output FORMAT           Output format (terminal, markdown, json)
  --no-inline               Skip inline diff comments
  --no-summary              Skip summary comment
  --verbose                 Enable debug output
```

## License

MIT
