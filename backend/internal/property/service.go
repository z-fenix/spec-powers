// Package property implements project-level custom property definitions
// (select / multi_select / checkbox / text / number / date) and the values
// issues carry for them.
package property

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

const (
	TypeSelect      = "select"
	TypeMultiSelect = "multi_select"
	TypeCheckbox    = "checkbox"
	TypeText        = "text"
	TypeNumber      = "number"
	TypeDate        = "date"
)

var validTypes = map[string]bool{
	TypeSelect: true, TypeMultiSelect: true, TypeCheckbox: true,
	TypeText: true, TypeNumber: true, TypeDate: true,
}

type Service struct {
	props    store.PropertyStore
	projects store.ProjectStore
	issues   store.IssueStore
}

func NewService(props store.PropertyStore, projects store.ProjectStore, issues store.IssueStore) *Service {
	return &Service{props: props, projects: projects, issues: issues}
}

type DefinitionInput struct {
	Name    string
	Type    string
	Options []string
}

// requireProjectRole enforces project-level access with the same semantics
// as the issue and project domains: unknown projects are 404, non-members
// 403.
func (s *Service) requireProjectRole(ctx context.Context, userID, projectID, minRole string) error {
	if _, err := s.projects.GetProject(ctx, projectID); err != nil {
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project failed")
	}
	pm, err := s.projects.GetProjectMember(ctx, projectID, userID)
	if err == store.ErrNotFound {
		return httpapi.ErrForbidden("not a project member")
	}
	if err != nil {
		return httpapi.ErrInternal("get project member failed")
	}
	if minRole == "owner" && pm.Role != "owner" {
		return httpapi.ErrForbidden("owner role required")
	}
	return nil
}

func validateDefinition(in DefinitionInput) (DefinitionInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return in, httpapi.ErrInvalid("property name is required")
	}
	if !validTypes[in.Type] {
		return in, httpapi.ErrInvalid("unknown property type: " + in.Type)
	}
	if in.Type == TypeSelect || in.Type == TypeMultiSelect {
		seen := map[string]bool{}
		for _, o := range in.Options {
			if strings.TrimSpace(o) == "" {
				return in, httpapi.ErrInvalid("select options must not be blank")
			}
			if seen[o] {
				return in, httpapi.ErrInvalid("duplicate select option: " + o)
			}
			seen[o] = true
		}
		if len(in.Options) == 0 {
			return in, httpapi.ErrInvalid("select properties require at least one option")
		}
	} else if len(in.Options) > 0 {
		return in, httpapi.ErrInvalid("options are only valid for select / multi_select")
	}
	return in, nil
}

func (s *Service) CreateDefinition(ctx context.Context, userID, projectID string, in DefinitionInput) (*domain.PropertyDefinition, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	in, err := validateDefinition(in)
	if err != nil {
		return nil, err
	}
	existing, err := s.props.ListPropertyDefinitions(ctx, projectID)
	if err != nil {
		return nil, httpapi.ErrInternal("list property definitions failed")
	}
	d := &domain.PropertyDefinition{
		ProjectID: projectID,
		Name:      in.Name,
		Type:      in.Type,
		Options:   in.Options,
		Position:  len(existing),
	}
	created, err := s.props.CreatePropertyDefinition(ctx, d)
	if err == store.ErrConflict {
		return nil, httpapi.ErrConflict("property name already exists")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("create property definition failed")
	}
	return created, nil
}

func (s *Service) ListDefinitions(ctx context.Context, userID, projectID string) ([]domain.PropertyDefinition, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	list, err := s.props.ListPropertyDefinitions(ctx, projectID)
	if err != nil {
		return nil, httpapi.ErrInternal("list property definitions failed")
	}
	return list, nil
}

func (s *Service) UpdateDefinition(ctx context.Context, userID, projectID, propertyID string, in DefinitionInput) (*domain.PropertyDefinition, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	current, err := s.props.GetPropertyDefinition(ctx, propertyID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("property definition not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get property definition failed")
	}
	if current.ProjectID != projectID {
		return nil, httpapi.ErrInvalid("property definition belongs to another project")
	}
	in, err = validateDefinition(in)
	if err != nil {
		return nil, err
	}
	current.Name, current.Type, current.Options = in.Name, in.Type, in.Options
	updated, err := s.props.UpdatePropertyDefinition(ctx, current)
	if err == store.ErrConflict {
		return nil, httpapi.ErrConflict("property name already exists")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("update property definition failed")
	}
	return updated, nil
}

func (s *Service) DeleteDefinition(ctx context.Context, userID, projectID, propertyID string) error {
	if err := s.requireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return err
	}
	current, err := s.props.GetPropertyDefinition(ctx, propertyID)
	if err == store.ErrNotFound {
		return httpapi.ErrNotFound("property definition not found")
	}
	if err != nil {
		return httpapi.ErrInternal("get property definition failed")
	}
	if current.ProjectID != projectID {
		return httpapi.ErrInvalid("property definition belongs to another project")
	}
	if err := s.props.DeletePropertyDefinition(ctx, propertyID); err != nil {
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("property definition not found")
		}
		return httpapi.ErrInternal("delete property definition failed")
	}
	return nil
}

// ValidateValue checks one canonical string value against a property type.
// multi_select values are JSON arrays of select options.
func ValidateValue(typ, value string, options []string) error {
	switch typ {
	case TypeText:
		return nil
	case TypeNumber:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return httpapi.ErrInvalid("value must be a number")
		}
	case TypeCheckbox:
		if value != "true" && value != "false" {
			return httpapi.ErrInvalid("checkbox value must be true or false")
		}
	case TypeDate:
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return httpapi.ErrInvalid("date value must be YYYY-MM-DD")
		}
	case TypeSelect:
		for _, o := range options {
			if o == value {
				return nil
			}
		}
		return httpapi.ErrInvalid("value is not one of the select options")
	case TypeMultiSelect:
		var picked []string
		if err := json.Unmarshal([]byte(value), &picked); err != nil {
			return httpapi.ErrInvalid("multi_select value must be a JSON array of options")
		}
		allowed := map[string]bool{}
		for _, o := range options {
			allowed[o] = true
		}
		for _, p := range picked {
			if !allowed[p] {
				return httpapi.ErrInvalid("value is not one of the select options")
			}
		}
	default:
		return httpapi.ErrInvalid("unknown property type: " + typ)
	}
	return nil
}

// requireProjectIssue loads the issue and enforces member access to its
// project.
func (s *Service) requireProjectIssue(ctx context.Context, userID, issueID string) (*domain.Issue, error) {
	i, err := s.issues.GetIssue(ctx, issueID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("issue not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get issue failed")
	}
	if err := s.requireProjectRole(ctx, userID, i.ProjectID, "member"); err != nil {
		return nil, err
	}
	return i, nil
}

// SetIssueValue assigns value to the issue for the property. An empty value
// clears the assignment instead.
func (s *Service) SetIssueValue(ctx context.Context, userID, issueID, propertyID, value string) (*domain.IssuePropertyValue, error) {
	i, err := s.requireProjectIssue(ctx, userID, issueID)
	if err != nil {
		return nil, err
	}
	d, err := s.props.GetPropertyDefinition(ctx, propertyID)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("property definition not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get property definition failed")
	}
	if d.ProjectID != i.ProjectID {
		return nil, httpapi.ErrInvalid("property definition belongs to another project")
	}
	if value == "" {
		if err := s.props.DeleteIssueProperty(ctx, issueID, propertyID); err != nil && err != store.ErrNotFound {
			return nil, httpapi.ErrInternal("delete issue property failed")
		}
		return &domain.IssuePropertyValue{IssueID: issueID, PropertyID: propertyID, Value: ""}, nil
	}
	if err := ValidateValue(d.Type, value, d.Options); err != nil {
		return nil, err
	}
	saved, err := s.props.SetIssueProperty(ctx, &domain.IssuePropertyValue{IssueID: issueID, PropertyID: propertyID, Value: value})
	if err != nil {
		return nil, httpapi.ErrInternal("set issue property failed")
	}
	return saved, nil
}

func (s *Service) ListIssueValues(ctx context.Context, userID, issueID string) ([]domain.IssuePropertyValue, error) {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return nil, err
	}
	list, err := s.props.ListIssueProperties(ctx, issueID)
	if err != nil {
		return nil, httpapi.ErrInternal("list issue properties failed")
	}
	return list, nil
}

// ListProjectValues returns every issue property value in the project —
// the board uses it to filter issues by property without per-issue reads.
func (s *Service) ListProjectValues(ctx context.Context, userID, projectID string) ([]domain.IssuePropertyValue, error) {
	if err := s.requireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	list, err := s.props.ListIssuePropertiesForProject(ctx, projectID)
	if err != nil {
		return nil, httpapi.ErrInternal("list project issue properties failed")
	}
	return list, nil
}

func (s *Service) DeleteIssueValue(ctx context.Context, userID, issueID, propertyID string) error {
	if _, err := s.requireProjectIssue(ctx, userID, issueID); err != nil {
		return err
	}
	if err := s.props.DeleteIssueProperty(ctx, issueID, propertyID); err != nil {
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("property value not found")
		}
		return httpapi.ErrInternal("delete issue property failed")
	}
	return nil
}
