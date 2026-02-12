package repository

import "testing"

func TestAnalyticsRepositoryNew(t *testing.T) {
	repo := NewAnalyticsRepository(nil)
	if repo == nil {
		t.Fatalf("repo is nil")
	}
}
