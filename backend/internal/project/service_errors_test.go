package project

import (
	"context"
	"errors"
	"testing"

	"specpowers/backend/internal/httpapi"
)

func TestServiceHandlesStoreFailure(t *testing.T) {
	projects := newFakeProjects()
	projects.failGet = true
	svc := NewService(projects, newFakeUsers(), newFakeMembers(), &fakeWorkspaces{})

	err := svc.RequireProjectRole(context.Background(), "u1", "p1", "member")
	var appErr *httpapi.AppError
	if !errors.As(err, &appErr) || appErr.Status != 500 {
		t.Errorf("store failure error = %v, want 500", err)
	}
}
