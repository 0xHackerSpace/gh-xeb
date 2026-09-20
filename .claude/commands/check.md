---
description: Run the full local gate (gofmt, go vet, go test) and fix what fails
allowed-tools: Bash(make check:*), Bash(gofmt:*), Bash(go test:*), Bash(go vet:*), Read, Edit
---

Run `make check`.

If it fails, fix the cause and re-run until it passes. Report the final state:
which step failed, what you changed, and the passing output. Do not report
success without a clean run.
