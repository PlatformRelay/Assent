package builtin

// Config type strings for the forge-groups builtin (example packs use
// builtin/gitlab-groups; forge-groups is the forge-neutral alias).
const (
	TypeGitlabGroups = "builtin/gitlab-groups"
	TypeForgeGroups  = "builtin/forge-groups"
)

// IsForgeGroupsType reports whether typ is the forge-groups builtin
// (gitlab-groups or forge-groups). Other builtins (repo-file, …) return false.
func IsForgeGroupsType(typ string) bool {
	switch typ {
	case TypeGitlabGroups, TypeForgeGroups:
		return true
	default:
		return false
	}
}

// IsResourceOwnerType reports whether typ is the resource→owner builtin.
func IsResourceOwnerType(typ string) bool {
	return typ == TypeResourceOwner
}
