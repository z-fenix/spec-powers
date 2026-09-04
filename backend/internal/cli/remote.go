package cli

import (
	"context"
	"fmt"

	"specpowers/backend/internal/agent"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
)

// remoteStores adapts the server's runtime endpoints to the store
// interfaces the agent executor needs, so the same LLM tool loop runs
// locally against a remote workspace. Only the operations the executor
// performs are implemented; the embedded interfaces make any other method
// a nil-pointer panic, which is the intended signal if the executor grows.
type remoteStores struct {
	store.IssueStore
	store.CommentStore
	store.IssueMetadataStore
	store.ProjectStore
	c *Client
	// lastContext caches the most recent issue context read; the checkout
	// tool resolves the issue's project resources from it (the executor
	// always reads the issue before checking out).
	lastContext *RunContext
}

func newRemoteStores(c *Client) *remoteStores {
	return &remoteStores{c: c}
}

var _ store.IssueStore = (*remoteStores)(nil)
var _ store.CommentStore = (*remoteStores)(nil)
var _ store.IssueMetadataStore = (*remoteStores)(nil)
var _ store.ProjectStore = (*remoteStores)(nil)

func (r *remoteStores) fetchContext(ctx context.Context, issueID string) (*RunContext, error) {
	rc, err := r.c.GetRunContext(issueID)
	if err != nil {
		return nil, err
	}
	r.lastContext = rc
	return rc, nil
}

func (r *remoteStores) GetIssue(ctx context.Context, id string) (*domain.Issue, error) {
	rc, err := r.fetchContext(ctx, id)
	if err != nil {
		return nil, err
	}
	i := rc.Issue
	return &domain.Issue{
		ID: i.ID, ProjectID: i.ProjectID, ParentID: i.ParentID,
		Title: i.Title, Description: i.Description, Status: i.Status, AssigneeID: i.AssigneeID,
	}, nil
}

// UpdateIssue carries only the status over the wire (the server validates
// the transition); the executor uses it for set_status.
func (r *remoteStores) UpdateIssue(ctx context.Context, i *domain.Issue) (*domain.Issue, error) {
	if err := r.c.SetIssueStatus(i.ID, i.Status); err != nil {
		return nil, err
	}
	return i, nil
}

func (r *remoteStores) ListComments(ctx context.Context, issueID string) ([]domain.IssueComment, error) {
	rc, err := r.fetchContext(ctx, issueID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.IssueComment, 0, len(rc.Comments))
	for _, c := range rc.Comments {
		out = append(out, domain.IssueComment{
			ID: c.ID, IssueID: issueID, ParentID: c.ParentID, AuthorID: c.AuthorID, Content: c.Content,
		})
	}
	return out, nil
}

func (r *remoteStores) CreateComment(ctx context.Context, c *domain.IssueComment) (*domain.IssueComment, error) {
	id, err := r.c.PostIssueComment(c.IssueID, c.Content, c.ParentID)
	if err != nil {
		return nil, err
	}
	out := *c
	out.ID = id
	return &out, nil
}

func (r *remoteStores) ListIssueMetadata(ctx context.Context, issueID string) ([]domain.IssueMetadata, error) {
	rc, err := r.fetchContext(ctx, issueID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.IssueMetadata, 0, len(rc.Metadata))
	for _, m := range rc.Metadata {
		out = append(out, domain.IssueMetadata{IssueID: issueID, Key: m.Key, Value: m.Value, Type: m.Type})
	}
	return out, nil
}

func (r *remoteStores) ListProjectResources(ctx context.Context, projectID string) ([]domain.ProjectResource, error) {
	rc := r.lastContext
	if rc == nil || rc.Issue.ProjectID != projectID {
		return nil, fmt.Errorf("checkout_repo: read the issue first (the runtime resolves resources from the issue context)")
	}
	out := make([]domain.ProjectResource, 0, len(rc.Resources))
	for _, res := range rc.Resources {
		out = append(out, domain.ProjectResource{
			ID: res.ID, ProjectID: projectID, Type: res.Type, Label: res.Label, Pointer: res.Pointer,
		})
	}
	return out, nil
}

// AppendRunLog implements the executor's logAppender.
func (r *remoteStores) AppendRunLog(ctx context.Context, l *domain.RunLog) (*domain.RunLog, error) {
	if err := r.c.AppendRunLog(l.RunID, l.Kind, l.Content); err != nil {
		return nil, err
	}
	return l, nil
}

// remoteFlow adapts the server's classic-flow (change) endpoints to the
// executor's FlowDriver; the agent's token identifies it server-side.
type remoteFlow struct {
	c *Client
}

var _ agent.FlowDriver = (*remoteFlow)(nil)

func toDomainChange(c *Change) *domain.Change {
	return &domain.Change{ID: c.ID, ProjectID: c.ProjectID, IssueID: c.IssueID, Phase: c.Phase, Status: c.Status}
}

func (f *remoteFlow) EnsureChange(ctx context.Context, userID, issueID string) (*domain.Change, error) {
	ch, err := f.c.GetChangeByIssue(issueID)
	if err == nil {
		return toDomainChange(ch), nil
	}
	if !NotFound(err) {
		return nil, err
	}
	ch, err = f.c.CreateChangeManual(issueID)
	if err != nil {
		return nil, err
	}
	return toDomainChange(ch), nil
}

func (f *remoteFlow) PhaseSkill(ctx context.Context, userID string, change *domain.Change) (*skill.Skill, error) {
	s, err := f.c.NextSkill(change.ID)
	if err != nil {
		return nil, err
	}
	return &skill.Skill{
		Key: s.Key, Name: s.Name, Description: s.Description, Order: s.Order, Instructions: s.Instructions,
	}, nil
}

func (f *remoteFlow) WriteArtifact(ctx context.Context, userID string, change *domain.Change, kind, content string) (*domain.Artifact, error) {
	a, err := f.c.WriteArtifact(change.ID, kind, content)
	if err != nil {
		return nil, err
	}
	return &domain.Artifact{ID: a.ID, ChangeID: a.ChangeID, Kind: a.Kind, Version: a.Version}, nil
}

func (f *remoteFlow) AdvancePhase(ctx context.Context, userID string, change *domain.Change) (*domain.Change, error) {
	c, _, err := f.c.AdvanceGuard(change.ID)
	if err != nil {
		return nil, err
	}
	return toDomainChange(c), nil
}

func (f *remoteFlow) SubmitVerify(ctx context.Context, userID string, change *domain.Change, content string) (*domain.Artifact, error) {
	a, _, err := f.c.SubmitVerifyReport(change.ID, content)
	if err != nil {
		return nil, err
	}
	return &domain.Artifact{ID: a.ID, ChangeID: a.ChangeID, Kind: a.Kind, Version: a.Version}, nil
}

func (f *remoteFlow) Archive(ctx context.Context, userID string, change *domain.Change) (*domain.Change, error) {
	c, err := f.c.Archive(change.ID)
	if err != nil {
		return nil, err
	}
	return toDomainChange(c), nil
}
