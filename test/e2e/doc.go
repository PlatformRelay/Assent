// Package e2e holds the L3 real-forge end-to-end tests, all gated behind the
// `e2e` build tag (see skeleton_test.go and test/e2e/README.md). Run them with
// `task e2e` (`go test -tags e2e ./test/e2e/...`); they are excluded from
// `task check`.
//
// This file carries NO build tag on purpose: it guarantees the package always
// has at least one buildable Go file so that `go build/vet/test ./...` (without
// `-tags e2e`) sees a normal, empty package instead of erroring with
// "build constraints exclude all Go files". The autonomous PR gate never boots
// the containerized GitLab (P4-E1-S09 / REQ-P4-E1-S09-02).
package e2e
