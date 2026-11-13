# Pull Request

## Description

Provide a clear summary of what changed and the rationale behind it.

## Related Issue

Link to the specific issue that this PR addresses

## Type of Change

- [ ] Bug Fix (non-breaking change fixing an issue)
- [ ] New feature (non-breaking addition of functionality)
- [ ] Breaking change (fix or feature that alters existing behavior or protocol)
- [ ] Documentation / CI maintenance

## Contributor Checklist

- [ ] All commits are signed off with Developer Certificate of Origin (git commit -s).
- [ ] Code has been formatted using `gofmt -s -w .`.
- [ ] `go vet ./...` passes without warnings.
- [ ] Concurrency and race tests pass: `go test -v -race -count=1 ./...`.
- [ ] Cross-platform compatibility verified (especially Windows vs POSIX file permissions/path if touching filesystem or OS interaction).
- [ ] Documentation updated (README, doc comments, AGENTS.md), if applicable.
