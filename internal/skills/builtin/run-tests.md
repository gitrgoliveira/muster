---
id: run-tests
name: Run Tests
desc: Run the project test suite and act on failures
category: code
icon: ✅
mcpServers: []
---
Run the project's tests (for Go, `go test -race ./...`) after making changes. Treat a failing or racy test as blocking: read the failure, fix the cause, and re-run until green before considering the step done.
