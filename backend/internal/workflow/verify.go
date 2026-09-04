package workflow

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// KindVerify is the verify-report artifact kind; its content is YAML.
const KindVerify = "verify"

// VerifyReport is the parsed form of a verify report artifact.
type VerifyReport struct {
	Result  string `yaml:"result"`
	Summary string `yaml:"summary"`
}

// ParseVerifyReport validates a verify report: the content must be a YAML
// mapping with result set to "pass" or "fail". Anything else is rejected.
func ParseVerifyReport(content string) (VerifyReport, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return VerifyReport{}, fmt.Errorf("not valid YAML: %w", err)
	}
	if raw == nil {
		return VerifyReport{}, fmt.Errorf("verify report must be a YAML mapping")
	}
	var report VerifyReport
	if err := yaml.Unmarshal([]byte(content), &report); err != nil {
		return VerifyReport{}, fmt.Errorf("not valid YAML: %w", err)
	}
	switch report.Result {
	case "pass", "fail":
		return report, nil
	case "":
		return VerifyReport{}, fmt.Errorf("result field is required")
	default:
		return VerifyReport{}, fmt.Errorf("result must be pass or fail, got %q", report.Result)
	}
}
