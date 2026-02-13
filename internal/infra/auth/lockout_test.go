package auth

import (
	"strings"
	"testing"
)

func TestNormalizeLogin(t *testing.T) {
	l := NewLockout(nil, 5, 0, 64, nil, nil)

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "trim and lower", in: "  User@Example.COM ", want: "user@example.com"},
		{name: "empty", in: "   ", wantErr: true},
		{name: "too long", in: strings.Repeat("a", 80), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := l.Normalize(tt.in)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("got=%q want=%q", got, tt.want)
			}
		})
	}
}
