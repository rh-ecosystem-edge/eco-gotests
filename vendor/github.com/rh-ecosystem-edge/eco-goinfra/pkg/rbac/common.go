package rbac

// allowedSubjectKinds returns a list of supported v1.Subject kinds.
func allowedSubjectKinds() []string {
	return []string{"ServiceAccount", "User", "Group"}
}
