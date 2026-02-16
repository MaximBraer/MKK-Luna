package service

import "testing"

func TestAllowedTaskFields(t *testing.T) {
	if !allowedTaskFields(RoleMember)["status"] {
		t.Fatalf("member must be able to update status")
	}
	if allowedTaskFields(RoleMember)["title"] {
		t.Fatalf("member must not be able to update title")
	}
	if !allowedTaskFields(RoleAdmin)["title"] {
		t.Fatalf("admin must be able to update title")
	}
}

func TestTaskFieldValidationHelpers(t *testing.T) {
	if !isKnownTaskField("status") {
		t.Fatalf("status should be known field")
	}
	if isKnownTaskField("unknown") {
		t.Fatalf("unknown should not be known field")
	}
	if !isValidStatus("todo") || isValidStatus("invalid") {
		t.Fatalf("status validation mismatch")
	}
	if !isValidPriority("medium") || isValidPriority("invalid") {
		t.Fatalf("priority validation mismatch")
	}
}
