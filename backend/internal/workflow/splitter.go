package workflow

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/store"
)

// Change statuses beyond the seeded active/archived pair.
const ChangeStatusFailed = "failed"

// issueCreator is the slice of the issue service the splitter uses to
// create the parsed sub-issues.
type issueCreator interface {
	CreateIssue(ctx context.Context, userID, projectID string, in issue.CreateInput) (*domain.Issue, error)
}

// contextLookup reads the project's free-form context for prompts.
type contextLookup interface {
	GetProjectContext(ctx context.Context, projectID string) (*domain.ProjectContext, error)
}

// SplitterDeps wires the splitter's collaborators.
type SplitterDeps struct {
	Client     llm.Client
	Changes    store.ChangeStore
	Artifacts  store.ArtifactStore
	Mappings   store.TaskMappingStore
	Issues     issueLookup
	Creator    issueCreator
	Contexts   contextLookup
	Templates  map[string]*template.Template
	MaxRetries int // extra attempts per phase after the first
}

// Splitter runs the classic flow (proposal → specs → design → tasks) for one
// issue: it generates each artifact with the LLM, stores it, and finally
// parses the tasks artifact into staged sub-issues bound to the change.
type Splitter struct {
	client     llm.Client
	changes    store.ChangeStore
	artifacts  store.ArtifactStore
	mappings   store.TaskMappingStore
	issues     issueLookup
	creator    issueCreator
	contexts   contextLookup
	templates  map[string]*template.Template
	maxRetries int
}

func NewSplitter(deps SplitterDeps) *Splitter {
	if deps.Templates == nil {
		deps.Templates = defaultTemplates()
	}
	return &Splitter{
		client:     deps.Client,
		changes:    deps.Changes,
		artifacts:  deps.Artifacts,
		mappings:   deps.Mappings,
		issues:     deps.Issues,
		creator:    deps.Creator,
		contexts:   deps.Contexts,
		templates:  deps.Templates,
		maxRetries: deps.MaxRetries,
	}
}

func phaseOrder() []string {
	return []string{KindProposal, KindSpecs, KindDesign, KindTasks}
}

// Run executes the whole classic flow synchronously and returns the change
// with its phase advanced to tasks.
func (s *Splitter) Run(ctx context.Context, userID, issueID string) (*domain.Change, error) {
	i, err := s.issues.GetIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get issue failed")
	}
	if _, err := s.changes.GetChangeByIssue(ctx, issueID); err == nil {
		return nil, httpapi.ErrConflict("change already exists for this issue")
	} else if err != store.ErrNotFound {
		return nil, httpapi.ErrInternal("get change failed")
	}

	projectContext := ""
	if s.contexts != nil {
		if pc, err := s.contexts.GetProjectContext(ctx, i.ProjectID); err == nil && pc != nil {
			projectContext = pc.Content
		}
	}

	change, err := s.changes.CreateChange(ctx, &domain.Change{
		ProjectID: i.ProjectID,
		IssueID:   issueID,
		Phase:     KindProposal,
		Status:    "active",
		CreatedBy: userID,
	})
	if err != nil {
		return nil, httpapi.ErrInternal("create change failed")
	}

	data := PromptData{
		IssueTitle:       i.Title,
		IssueDescription: i.Description,
		ProjectContext:   projectContext,
		Artifacts:        map[string]string{},
	}

	order := phaseOrder()
	for idx, kind := range order {
		content, err := s.generatePhase(ctx, kind, data)
		if err != nil {
			s.failChange(ctx, change)
			return nil, httpapi.ErrInternal(fmt.Sprintf("generate %s failed: %v", kind, err))
		}
		saved, err := s.artifacts.CreateArtifact(ctx, &domain.Artifact{
			ChangeID:  change.ID,
			Kind:      kind,
			Content:   content,
			CreatedBy: userID,
		})
		if err != nil {
			s.failChange(ctx, change)
			return nil, httpapi.ErrInternal("save artifact failed")
		}
		data.Artifacts[kind] = content

		if kind == KindTasks {
			if err := s.createSubIssues(ctx, userID, i, change, saved, content); err != nil {
				s.failChange(ctx, change)
				return nil, err
			}
		} else if idx+1 < len(order) {
			if _, err := s.changes.CreateChangeHandoff(ctx, &domain.ChangeHandoff{
				ChangeID:  change.ID,
				FromPhase: kind,
				ToPhase:   order[idx+1],
				CreatedBy: userID,
			}); err != nil {
				s.failChange(ctx, change)
				return nil, httpapi.ErrInternal("record handoff failed")
			}
			change.Phase = order[idx+1]
			if _, err := s.changes.UpdateChange(ctx, change); err != nil {
				s.failChange(ctx, change)
				return nil, httpapi.ErrInternal("advance change phase failed")
			}
		}
	}
	return change, nil
}

// generatePhase renders the phase prompt and calls the LLM until the output
// passes validation or attempts run out.
func (s *Splitter) generatePhase(ctx context.Context, kind string, data PromptData) (string, error) {
	prompt, err := RenderPrompt(s.templates, kind, data)
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		completion, err := s.client.Complete(ctx, "", prompt)
		if err != nil {
			lastErr = err
			continue
		}
		if err := validateArtifact(kind, completion.Text); err != nil {
			lastErr = err
			continue
		}
		return completion.Text, nil
	}
	return "", lastErr
}

// validateArtifact enforces per-kind output quality before persisting.
func validateArtifact(kind, content string) error {
	if kind == KindTasks {
		_, err := ParseTasks(content)
		return err
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("empty %s output", kind)
	}
	return nil
}

// createSubIssues turns the parsed tasks list into sub-issues under the
// parent issue and binds them to the tasks artifact via task mappings.
func (s *Splitter) createSubIssues(ctx context.Context, userID string, parent *domain.Issue, change *domain.Change, tasksArtifact *domain.Artifact, content string) error {
	return bindTaskSubIssuesTo(ctx, s.creator, s.mappings, userID, parent, change, tasksArtifact, content)
}

// bindTaskSubIssuesTo is the shared tasks-phase mechanics: parse the tasks
// JSON, create one sub-issue per task under the parent, and replace the
// change's task mappings with entries bound to the tasks artifact. It backs
// both the AI splitter and the manual artifact write.
func bindTaskSubIssuesTo(ctx context.Context, creator issueCreator, mappings store.TaskMappingStore, userID string, parent *domain.Issue, change *domain.Change, tasksArtifact *domain.Artifact, content string) error {
	specs, err := ParseTasks(content)
	if err != nil {
		return httpapi.ErrInvalid("parse tasks artifact failed: " + err.Error())
	}
	items := make([]domain.TaskMapping, 0, len(specs))
	for pos, spec := range specs {
		created, err := creator.CreateIssue(ctx, userID, parent.ProjectID, issue.CreateInput{
			Title:       spec.Title,
			Description: spec.Description,
			ParentID:    parent.ID,
			Stage:       spec.Stage,
		})
		if err != nil {
			return err
		}
		items = append(items, domain.TaskMapping{
			ChangeID:   change.ID,
			ArtifactID: tasksArtifact.ID,
			IssueID:    created.ID,
			Title:      spec.Title,
			Stage:      spec.Stage,
			Position:   pos,
		})
	}
	if err := mappings.SetTaskMappings(ctx, change.ID, tasksArtifact.ID, items); err != nil {
		return httpapi.ErrInternal("save task mappings failed")
	}
	return nil
}

func (s *Splitter) failChange(ctx context.Context, change *domain.Change) {
	change.Status = ChangeStatusFailed
	if _, err := s.changes.UpdateChange(ctx, change); err != nil {
		// the primary error matters more; a failed status write is best effort
		_ = err
	}
}
