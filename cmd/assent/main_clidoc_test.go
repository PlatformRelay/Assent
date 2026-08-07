package main

import (
	"os"
	"strings"
	"testing"
)

// cliDocPath is the published CLI reference the help surface is pinned against
// (REQ-AUD-S05-02). Relative to this package directory.
const cliDocPath = "../../docs/usage/cli.md"

// REQ-AUD-S05-02: docs/usage/cli.md documents every dispatched subcommand and
// embeds the binary's help output verbatim, so the page cannot drift from the tool.
func TestCLIDocCoversSubcommands(t *testing.T) {
	doc := readCLIDoc(t)
	table := subcommands()

	if missing := missingDocSections(doc, table); len(missing) != 0 {
		t.Errorf("docs/usage/cli.md has no section for %v", missing)
	}
	if extra := extraDocSections(doc, table); len(extra) != 0 {
		t.Errorf("docs/usage/cli.md documents %v, which the CLI does not dispatch", extra)
	}
	if !strings.Contains(doc, usageText()) {
		t.Errorf("docs/usage/cli.md does not embed the verbatim help output; expected block:\n---\n%s", usageText())
	}
	for _, sc := range table {
		if !strings.Contains(doc, sc.usage) {
			t.Errorf("docs/usage/cli.md omits the usage line for %q: %s", sc.name, sc.usage)
		}
	}
}

// Negative polarity for the doc-drift check: an undocumented subcommand is
// reported, and a documented-but-undispatched section is reported too.
func TestCLIDocDriftIsDetected(t *testing.T) {
	doc := readCLIDoc(t)
	table := subcommands()

	withGhost := make([]subcommand, 0, len(table)+1)
	withGhost = append(withGhost, table...)
	withGhost = append(withGhost, subcommand{name: "ghost-command", synopsis: "s", usage: "u"})
	if missing := missingDocSections(doc, withGhost); len(missing) != 1 || missing[0] != "ghost-command" {
		t.Fatalf("missingDocSections = %v, want exactly [ghost-command]", missing)
	}

	if extra := extraDocSections(doc, table[1:]); len(extra) != 1 || extra[0] != table[0].name {
		t.Fatalf("extraDocSections = %v, want exactly [%s]", extra, table[0].name)
	}
}

func readCLIDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(cliDocPath)
	if err != nil {
		t.Fatalf("read %s: %v", cliDocPath, err)
	}
	return string(data)
}

// missingDocSections returns the subcommands without a `## assent <name>` section.
func missingDocSections(doc string, table []subcommand) []string {
	sections := docSections(doc)
	var missing []string
	for _, sc := range table {
		if !sections[sc.name] {
			missing = append(missing, sc.name)
		}
	}
	return missing
}

// extraDocSections returns doc sections that no dispatched subcommand backs — a
// page describing a command the binary does not have is the same class of drift.
func extraDocSections(doc string, table []subcommand) []string {
	dispatched := make(map[string]bool, len(table))
	for _, sc := range table {
		dispatched[sc.name] = true
	}
	var extra []string
	for name := range docSections(doc) {
		if !dispatched[name] {
			extra = append(extra, name)
		}
	}
	return extra
}

// docSections collects the `## assent <name>` headings of the CLI reference.
func docSections(doc string) map[string]bool {
	const prefix = "## assent "
	found := map[string]bool{}
	for _, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if name := strings.TrimSpace(strings.TrimPrefix(line, prefix)); name != "" {
			found[name] = true
		}
	}
	return found
}
