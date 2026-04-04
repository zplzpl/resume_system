package rbac

type Role string

const (
	RoleSuperAdmin  Role = "super_admin"
	RoleHR          Role = "hr"
	RoleInterviewer Role = "interviewer"
)

type Action string

const (
	ActionCandidateRead   Action = "candidate:read"
	ActionCandidateWrite  Action = "candidate:write"
	ActionInterviewManage Action = "interview:manage"
	ActionUserManage      Action = "user:manage"
)

var rolePermissions = map[Role]map[Action]struct{}{
	RoleSuperAdmin: {
		ActionCandidateRead:   {},
		ActionCandidateWrite:  {},
		ActionInterviewManage: {},
		ActionUserManage:      {},
	},
	RoleHR: {
		ActionCandidateRead:   {},
		ActionCandidateWrite:  {},
		ActionInterviewManage: {},
	},
	RoleInterviewer: {
		ActionCandidateRead:   {},
		ActionInterviewManage: {},
	},
}

func IsKnownRole(role Role) bool {
	_, ok := rolePermissions[role]
	return ok
}

func Can(role Role, action Action) bool {
	actions, ok := rolePermissions[role]
	if !ok {
		return false
	}
	_, ok = actions[action]
	return ok
}

func Permissions(role Role) []Action {
	actions, ok := rolePermissions[role]
	if !ok {
		return nil
	}
	out := make([]Action, 0, len(actions))
	for action := range actions {
		out = append(out, action)
	}
	return out
}
