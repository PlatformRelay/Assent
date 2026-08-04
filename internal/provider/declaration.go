package provider

// DeclarationsEqual reports whether two declarations match on every
// cross-checked field (type/cardinality/subject/sensitive/maxAge).
func DeclarationsEqual(a, b Declaration) bool {
	return a.Type == b.Type &&
		a.Cardinality == b.Cardinality &&
		a.Subject == b.Subject &&
		a.Sensitive == b.Sensitive &&
		a.MaxAge == b.MaxAge
}
