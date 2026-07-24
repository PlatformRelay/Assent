# locked: D-012 — contract unlocked (D-017/E11), implementation gated until the Phase-4 adoption gate; do not wire into assent test or the schema-validation CI job before then
# Rego as the escape-hatch predicate backend inside the YAML envelope (ADR-0002 v2): the
# envelope owns match/effect/points; the module only computes violations over PolicyInput.
# Shown standalone here for readability; the same archetype in pure envelope syntax lives
# in ../declarative/bounded-change.yaml.
package policies.bounded_change

import rego.v1

# input.changes: [{path, file, kind, old, new}]  (canonical ChangeSet, ADR-0003)
# input.facts:   provider results, e.g. quotas   (ADR-0004)

violations contains v if {
	some change in input.changes
	change.kind == "modify"
	endswith(change.path, "/partitions")
	change.new < change.old
	v := {
		"path": change.path,
		"message": sprintf("partitions may not decrease (%d -> %d)", [change.old, change.new]),
	}
}

violations contains v if {
	some change in input.changes
	change.kind == "modify"
	endswith(change.path, "/partitions")
	change.new > input.facts.quota.max_partitions
	v := {
		"path": change.path,
		"message": sprintf("partitions %d exceeds quota %d", [change.new, input.facts.quota.max_partitions]),
	}
}
