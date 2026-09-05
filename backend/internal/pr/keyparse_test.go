package pr

import (
	"reflect"
	"testing"
)

func TestExtractIssueKeys(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "title with key", in: "feat: issue ↔ PR 关联与 close intent (SP-44)", want: []string{"SP-44"}},
		{name: "multiple keys deduped", in: "SP-4 and SP-44, plus SP-4 again", want: []string{"SP-4", "SP-44"}},
		{name: "branch name", in: "agent/kuncoding/SP-44-fix-thing", want: []string{"SP-44"}},
		{name: "lowercase ignored in bare text", in: "see issue sp-44 and issue-7", want: nil},
		{name: "no key", in: "no references here", want: nil},
		{name: "digits only dash", in: "v1-2 and 3-44 are not keys", want: nil},
		{name: "key with underscore prefix", in: "MULTI_WORD-12 ok", want: []string{"MULTI_WORD-12"}},
		{name: "too long prefix ignored", in: "SIXTEENCHARPREFIXX-1 no", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractIssueKeys(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ExtractIssueKeys(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseCloseIntents(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "closes", in: "Closes SP-44", want: []string{"SP-44"}},
		{name: "fixes lowercase", in: "fixes sp-7", want: []string{"SP-7"}},
		{name: "resolved with colon", in: "resolved: SP-9", want: []string{"SP-9"}},
		{name: "multiple", in: "Closes SP-1, fixes SP-2", want: []string{"SP-1", "SP-2"}},
		{name: "bare key is not intent", in: "relates to SP-44", want: nil},
		{name: "keyword inside word ignored", in: "prefix SP-44", want: nil},
		{name: "merged branch mention", in: "Merge PR for SP-44 (closes SP-44)", want: []string{"SP-44"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCloseIntents(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseCloseIntents(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitIssueKey(t *testing.T) {
	prefix, n := SplitIssueKey("SP-44")
	if prefix != "SP" || n != 44 {
		t.Errorf("SplitIssueKey(SP-44) = %q, %d", prefix, n)
	}
	if p, n := SplitIssueKey("not-a-key"); p != "" || n != 0 {
		t.Errorf("SplitIssueKey(invalid) = %q, %d", p, n)
	}
}
