# Contributing Guidelines

## Code of Conduct

All contributors of this repository are expected to abide by the project's Code of Conduct, which can be found in the file named CODE_OF_CONDUCT.md.

## Developer Certificate of Origin

All commits to this repository must be signed off via:

`git commit -s -m "..."`

## Code Testing

Prior to submitting a pull request for this repository, contributors must test their code locally.

This (at a minimum) should include running the following commands:

```bash
# Code Formatting Checks
gofmt -s -w .

# Static Analysis
go vet ./...

# Concurrency and Race Detection
go test -v -race -count=1 ./...
```

Note:

> Any code that touches the filesystem must take into consideration that POSIX and Windows file permissions are represented by different permission codes. The code should be guarded with checks for operating system specific builds to ensure that permissions are applied correctly.

## Pull Requests

When creating a pull request, the following requirements must be met:
- Fork the repo and create a PR-specific branch.
- Each commit should be focused on a specific change or fix with a clear description of what it does.
- Provide a link to the relevant issue(s)
- Must pass the multi-OS Github Actions matrix.

## Bug Reports & Feature Suggestions

When submitting a bug report or feature suggestion, use the issue templates on github, or the ones provided at .github/ISSUE_TEMPLATE/
