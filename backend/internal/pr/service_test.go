package pr

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/store"
)

type fakePRs struct {
	byID          map[string]*domain.PullRequest
	byProject     map[string]*domain.PullRequest // "project|repo|number"
	links         map[string][]string            // prID -> issue IDs, in link order
	nextID        int
	failUpsert    bool
}

func newFakePRs() *fakePRs {
	return &fakePRs{
		byID:      map[string]*domain.PullRequest{},
		byProject: map[string]*domain.PullRequest{},
		links:     map[string][]string{},
	}
}

func prKey(projectID, repo string, number int64) string {
	return projectID + "|" + repo + "|" + string(rune('0'+number))
}

func (f *fakePRs) UpsertPullRequest(_ context.Context, pr *domain.PullRequest) (*domain.PullRequest, error) {
	if f.failUpsert {
		return nil, store.ErrConflict
	}
	key := prKey(pr.ProjectID, pr.Repo, pr.Number)
	if existing, ok := f.byProject[key]; ok {
		existing.Title = pr.Title
		existing.Body = pr.Body
		existing.HeadBranch = pr.HeadBranch
		clone := *existing
		f.byID[existing.ID] = &clone
		return &clone, nil
	}
	f.nextID++
	stored := *pr
	stored.ID = string(rune('P' + f.nextID))
	stored.State = pr.State
	if stored.State == "" {
		stored.State = StateOpen
	}
	f.byID[stored.ID] = &stored
	f.byProject[key] = &stored
	clone := stored
	return &clone, nil
}

func (f *fakePRs) GetPullRequest(_ context.Context, id string) (*domain.PullRequest, error) {
	pr, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *pr
	return &clone, nil
}

func (f *fakePRs) GetPullRequestByProjectNumber(_ context.Context, projectID, repo string, number int64) (*domain.PullRequest, error) {
	pr, ok := f.byProject[prKey(projectID, repo, number)]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *pr
	return &clone, nil
}

func (f *fakePRs) UpdatePullRequestState(_ context.Context, id, state string, mergedAt *time.Time) (*domain.PullRequest, error) {
	pr, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	pr.State = state
	if mergedAt != nil {
		pr.MergedAt = mergedAt
	}
	clone := *pr
	return &clone, nil
}

func (f *fakePRs) LinkIssue(_ context.Context, pullRequestID, issueID string) error {
	for _, id := range f.links[pullRequestID] {
		if id == issueID {
			return nil
		}
	}
	f.links[pullRequestID] = append(f.links[pullRequestID], issueID)
	return nil
}

func (f *fakePRs) ListPullRequestsForIssue(_ context.Context, issueID string) ([]domain.PullRequest, error) {
	var list []domain.PullRequest
	for _, pr := range f.byID {
		for _, iid := range f.links[pr.ID] {
			if iid == issueID {
				list = append(list, *pr)
			}
		}
	}
	return list, nil
}

func (f *fakePRs) ListLinkedIssues(_ context.Context, pullRequestID string) ([]string, error) {
	return f.links[pullRequestID], nil
}

type fakeIssues struct {
	byID     map[string]*domain.Issue
	byNumber map[string]*domain.Issue // "project|number"
}

func newFakeIssues() *fakeIssues {
	return &fakeIssues{byID: map[string]*domain.Issue{}, byNumber: map[string]*domain.Issue{}}
}

func (f *fakeIssues) add(i *domain.Issue) *domain.Issue {
	f.byID[i.ID] = i
	f.byNumber[i.ProjectID+"|"+string(rune('0'+i.Number))] = i
	return i
}

func (f *fakeIssues) GetIssue(_ context.Context, id string) (*domain.Issue, error) {
	i, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *i
	return &clone, nil
}

func (f *fakeIssues) GetIssueByNumber(_ context.Context, projectID string, number int64) (*domain.Issue, error) {
	i, ok := f.byNumber[projectID+"|"+string(rune('0'+number))]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *i
	return &clone, nil
}

func (f *fakeIssues) UpdateIssue(_ context.Context, i *domain.Issue) (*domain.Issue, error) {
	if _, ok := f.byID[i.ID]; !ok {
		return nil, store.ErrNotFound
	}
	clone := *i
	f.byID[i.ID] = &clone
	f.byNumber[i.ProjectID+"|"+string(rune('0'+i.Number))] = &clone
	return &clone, nil
}

type fakeProjects struct {
	byID     map[string]*domain.Project
	byKey    map[string]*domain.Project // "workspace|key"
	members  map[string]string          // "project|user" -> role
}

func newFakeProjects() *fakeProjects {
	return &fakeProjects{byID: map[string]*domain.Project{}, byKey: map[string]*domain.Project{}, members: map[string]string{}}
}

func (f *fakeProjects) add(p *domain.Project) { f.byID[p.ID] = p }
func (f *fakeProjects) GetProject(_ context.Context, id string) (*domain.Project, error) {
	p, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *p
	return &clone, nil
}

func (f *fakeProjects) GetProjectByKey(_ context.Context, workspaceID, key string) (*domain.Project, error) {
	p, ok := f.byKey[workspaceID+"|"+key]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *p
	return &clone, nil
}

func (f *fakeProjects) GetProjectMember(_ context.Context, projectID, userID string) (*domain.ProjectMember, error) {
	role, ok := f.members[projectID+"|"+userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &domain.ProjectMember{ProjectID: projectID, UserID: userID, Role: role}, nil
}

type fakeEvents struct {
	events []domain.IssueEvent
}

func (f *fakeEvents) CreateIssueEvent(_ context.Context, e *domain.IssueEvent) (*domain.IssueEvent, error) {
	f.events = append(f.events, *e)
	return e, nil
}

func (f *fakeEvents) ListIssueEvents(_ context.Context, issueID string) ([]domain.IssueEvent, error) {
	var out []domain.IssueEvent
	for _, e := range f.events {
		if e.IssueID == issueID {
			out = append(out, e)
		}
	}
	return out, nil
}

type fixture struct {
	svc     *Service
	prs     *fakePRs
	issues  *fakeIssues
	projects *fakeProjects
	events  *fakeEvents
}

func newFixture() *fixture {
	prs := newFakePRs()
	issues := newFakeIssues()
	projects := newFakeProjects()
	events := &fakeEvents{}
	wsProject := &domain.Project{ID: "proj-1", WorkspaceID: "ws-1", Key: "SP"}
	projects.add(wsProject)
	projects.byKey["ws-1|SP"] = wsProject
	other := &domain.Project{ID: "proj-2", WorkspaceID: "ws-1", Key: "XX"}
	projects.add(other)
	projects.byKey["ws-1|XX"] = other
	projects.members["proj-1|u1"] = "member"
	projects.members["proj-2|u1"] = "member"
	issues.add(&domain.Issue{ID: "i-44", ProjectID: "proj-1", Number: 44, Status: issue.StatusInProgress})
	issues.add(&domain.Issue{ID: "i-7", ProjectID: "proj-2", Number: 7, Status: issue.StatusTodo})
	svc := NewService(prs, issues, projects).WithEventStore(events)
	return &fixture{svc: svc, prs: prs, issues: issues, projects: projects, events: events}
}

func statusOf(t *testing.T, f *fixture, id string) string {
	t.Helper()
	i, err := f.issues.GetIssue(context.Background(), id)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	return i.Status
}

func TestUpsertLinksIssuesFromTitleBodyAndBranch(t *testing.T) {
	f := newFixture()
	pr, linked, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", UpsertInput{
		Repo:       "z-fenix/spec-powers",
		Number:     12,
		Title:      "feat: S4-2 (SP-44)",
		Body:       "Also touches XX-7.",
		HeadBranch: "agent/kuncoding/SP-44-x",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if pr.State != StateOpen {
		t.Errorf("state = %q, want open", pr.State)
	}
	if len(linked) != 2 || linked[0] != "SP-44" || linked[1] != "XX-7" {
		t.Errorf("linked = %v", linked)
	}
	got, err := f.prs.ListLinkedIssues(context.Background(), pr.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("stored links = %v, %v", got, err)
	}
}

func TestUpsertReupsertIsIdempotent(t *testing.T) {
	f := newFixture()
	in := UpsertInput{Repo: "r", Number: 1, Title: "fix SP-44", HeadBranch: "b/SP-44"}
	if _, _, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", in); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	pr, linked, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", in)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, _ := f.prs.ListLinkedIssues(context.Background(), pr.ID)
	if len(got) != 1 || got[0] != "i-44" {
		t.Errorf("links duplicated: %v", got)
	}
	if len(linked) != 1 {
		t.Errorf("linked = %v", linked)
	}
}

func TestUnresolvableKeysAreSkipped(t *testing.T) {
	f := newFixture()
	_, linked, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", UpsertInput{
		Repo: "r", Number: 2, Title: "unknown refs ZZ-99 and XX-9999",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(linked) != 0 {
		t.Errorf("linked = %v, want none", linked)
	}
}

func TestMergeAppliesCloseIntent(t *testing.T) {
	f := newFixture()
	pr, _, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", UpsertInput{
		Repo: "r", Number: 3, Title: "feat", Body: "Closes SP-44. Relates to XX-7.",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := f.svc.UpdatePullRequestState(context.Background(), "u1", pr.ID, StateMerged); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := statusOf(t, f, "i-44"); got != issue.StatusDone {
		t.Errorf("close-intent issue status = %q, want done", got)
	}
	if got := statusOf(t, f, "i-7"); got != issue.StatusTodo {
		t.Errorf("reference-only issue status = %q, want unchanged", got)
	}
	if len(f.events.events) != 1 || f.events.events[0].IssueID != "i-44" ||
		f.events.events[0].Field != "status" || f.events.events[0].OldValue != issue.StatusInProgress ||
		f.events.events[0].NewValue != issue.StatusDone {
		t.Errorf("events = %+v", f.events.events)
	}
}

func TestMergeViaUpsertAppliesCloseIntentOnce(t *testing.T) {
	f := newFixture()
	in := UpsertInput{Repo: "r", Number: 4, Title: "fixes SP-44", State: StateMerged}
	if _, _, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := statusOf(t, f, "i-44"); got != issue.StatusDone {
		t.Fatalf("status = %q, want done", got)
	}
	// Re-upserting the merged PR must not re-close or re-record.
	if _, _, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", in); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if len(f.events.events) != 1 {
		t.Errorf("events = %d, want 1", len(f.events.events))
	}
}

func TestMergeSkipsTerminalIssues(t *testing.T) {
	f := newFixture()
	f.issues.byID["i-44"].Status = issue.StatusCancelled
	_, _, err := f.svc.UpsertPullRequest(context.Background(), "u1", "proj-1", UpsertInput{
		Repo: "r", Number: 5, Title: "closes SP-44", State: StateMerged,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := statusOf(t, f, "i-44"); got != issue.StatusCancelled {
		t.Errorf("status = %q, want cancelled untouched", got)
	}
}

func TestUpsertValidation(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	if _, _, err := f.svc.UpsertPullRequest(ctx, "u1", "proj-1", UpsertInput{Number: 0, Title: "t"}); err == nil {
		t.Error("zero number accepted")
	}
	if _, _, err := f.svc.UpsertPullRequest(ctx, "u1", "proj-1", UpsertInput{Number: 1, Title: "  "}); err == nil {
		t.Error("blank title accepted")
	}
	if _, _, err := f.svc.UpsertPullRequest(ctx, "u1", "proj-1", UpsertInput{Number: 1, Title: "t", State: "zombie"}); err == nil {
		t.Error("bad state accepted")
	}
	if _, _, err := f.svc.UpsertPullRequest(ctx, "u1", "nope", UpsertInput{Number: 1, Title: "t"}); err == nil {
		t.Error("unknown project accepted")
	}
	_, _, err := f.svc.UpsertPullRequest(ctx, "stranger", "proj-1", UpsertInput{Number: 1, Title: "t"})
	var appErr *httpapi.AppError
	if err == nil {
		t.Fatal("non-member accepted")
	}
	if !errors.As(err, &appErr) || appErr.Status != http.StatusForbidden {
		t.Errorf("err = %v, want 403", err)
	}
}

func TestUpdateStateValidation(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	pr, _, err := f.svc.UpsertPullRequest(ctx, "u1", "proj-1", UpsertInput{Repo: "r", Number: 6, Title: "t"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := f.svc.UpdatePullRequestState(ctx, "u1", pr.ID, "weird"); err == nil {
		t.Error("bad state accepted")
	}
	if _, err := f.svc.UpdatePullRequestState(ctx, "u1", "missing", StateOpen); err == nil {
		t.Error("unknown PR accepted")
	}
}
