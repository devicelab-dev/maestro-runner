# GitHub Copilot Instructions

## Shell Commands

Always prefix terminal commands with `rtk` instead of calling them directly.

Examples:
- `rtk git status` instead of `git status`
- `rtk git log --oneline -10` instead of `git log --oneline -10`
- `rtk go test ./...` instead of `go test ./...`
- `rtk make test` instead of `make test`

This applies to all shell commands run via the terminal tool.
