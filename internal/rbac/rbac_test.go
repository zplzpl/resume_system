package rbac

import "testing"

func TestRoleMatrix(t *testing.T) {
	tests := []struct {
		name   string
		role   Role
		action Action
		want   bool
	}{
		{"super admin can manage users", RoleSuperAdmin, ActionUserManage, true},
		{"hr cannot manage users", RoleHR, ActionUserManage, false},
		{"hr can create candidate", RoleHR, ActionCandidateWrite, true},
		{"interviewer cannot create candidate", RoleInterviewer, ActionCandidateWrite, false},
		{"interviewer can manage interview", RoleInterviewer, ActionInterviewManage, true},
		{"unknown role denied", Role("guest"), ActionCandidateRead, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Can(tt.role, tt.action); got != tt.want {
				t.Fatalf("Can(%s, %s)=%v, want %v", tt.role, tt.action, got, tt.want)
			}
		})
	}
}
