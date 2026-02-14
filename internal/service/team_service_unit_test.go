package service

import "testing"

func TestCanInvite(t *testing.T) {
	tests := []struct {
		name     string
		inviter  string
		target   string
		expected bool
	}{
		{name: "owner can invite admin", inviter: RoleOwner, target: RoleAdmin, expected: true},
		{name: "owner can invite member", inviter: RoleOwner, target: RoleMember, expected: true},
		{name: "admin can invite member", inviter: RoleAdmin, target: RoleMember, expected: true},
		{name: "admin cannot invite admin", inviter: RoleAdmin, target: RoleAdmin, expected: false},
		{name: "member cannot invite", inviter: RoleMember, target: RoleMember, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canInvite(tt.inviter, tt.target); got != tt.expected {
				t.Fatalf("canInvite(%q, %q) = %v, want %v", tt.inviter, tt.target, got, tt.expected)
			}
		})
	}
}
