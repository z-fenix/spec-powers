package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"specpowers/backend/internal/skill"
)

const defaultServer = "http://localhost:8080"

const usage = `sp - spec-powers command line client

Usage:

  sp login  --server URL --email EMAIL --password PW [--register]
  sp open   --issue ID [--manual]
  sp skills
  sp skill  <KEY>
  sp skill  install [KEY...] [--dir PATH] [--agent claude-code|generic] [--force]
  sp skill  list --installed [--dir PATH] [--agent claude-code|generic]
  sp skill  uninstall KEY... [--dir PATH] [--agent claude-code|generic]
  sp next-skill [--change ID]
  sp artifact write <KIND> [--file PATH | --content TEXT] [--change ID]
  sp guard  [--change ID]
  sp handoff [--change ID]
  sp state record-check <build|verify> --command CMD --exit-code N [--cwd DIR] [--change ID]
  sp verify [--change ID] [--file PATH | --content YAML]
  sp archive [--change ID]

Global flags (usable on any command): --server URL, --token JWT, --json.
The server and token default to SP_SERVER / SP_TOKEN, then .specpower/session.json.

Exit codes: 0 success, 1 operation failed (gate or API error), 2 usage error.`

// Run executes one sp command and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "login":
		return cmdLogin(args[1:], stdout, stderr)
	case "open":
		return cmdOpen(args[1:], stdout, stderr)
	case "skills":
		return cmdSkills(args[1:], stdout, stderr)
	case "skill":
		if len(args) >= 2 {
			switch args[1] {
			case "install":
				return cmdSkillInstall(args[2:], stdout, stderr)
			case "list":
				return cmdSkillList(args[2:], stdout, stderr)
			case "uninstall":
				return cmdSkillUninstall(args[2:], stdout, stderr)
			}
		}
		return cmdSkill(args[1:], stdout, stderr)
	case "next-skill":
		return cmdNextSkill(args[1:], stdout, stderr)
	case "artifact":
		if len(args) >= 3 && args[1] == "write" {
			return cmdArtifactWrite(args[2:], stdout, stderr)
		}
		fmt.Fprintln(stderr, usage)
		return 2
	case "guard":
		return cmdGuard(args[1:], stdout, stderr)
	case "handoff":
		return cmdHandoff(args[1:], stdout, stderr)
	case "state":
		if len(args) >= 2 && args[1] == "record-check" {
			return cmdRecordCheck(args[2:], stdout, stderr)
		}
		fmt.Fprintln(stderr, usage)
		return 2
	case "verify":
		return cmdVerify(args[1:], stdout, stderr)
	case "archive":
		return cmdArchive(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s\n", args[0], usage)
		return 2
	}
}

// ---- shared plumbing ----

type env struct {
	stdout, stderr io.Writer
	json           bool
	server         string
	token          string
}

// failw prints a single error line to stderr and returns the exit code.
func (e *env) fail(code int, err error) int {
	fmt.Fprintln(e.stderr, "error:", err)
	return code
}

// resolveFlags parses the global flags shared by every command.
func (e *env) resolveFlags(fs *flag.FlagSet) {
	fs.StringVar(&e.server, "server", "", "server base URL (default SP_SERVER, session, "+defaultServer+")")
	fs.StringVar(&e.token, "token", "", "bearer token (default SP_TOKEN, session)")
	fs.BoolVar(&e.json, "json", false, "emit machine-readable JSON")
}

// connection resolves server/token and builds the client. A token is
// required for every API command.
func (e *env) connection() (*Client, error) {
	sess, err := LoadSession()
	if err != nil {
		return nil, err
	}
	server := e.server
	if server == "" {
		server = os.Getenv("SP_SERVER")
	}
	if server == "" {
		server = sess.Server
	}
	if server == "" {
		server = defaultServer
	}
	token := e.token
	if token == "" {
		token = os.Getenv("SP_TOKEN")
	}
	if token == "" {
		token = sess.Token
	}
	if token == "" {
		return nil, fmt.Errorf("not logged in: run `sp login` or set SP_TOKEN")
	}
	return New(server, token), nil
}

// requireChange resolves the target change from --change or the local state.
func (e *env) requireChange(changeFlag string) (string, error) {
	if changeFlag != "" {
		return changeFlag, nil
	}
	st, err := LoadState()
	if err != nil {
		return "", err
	}
	if st.ChangeID == "" {
		return "", fmt.Errorf("no change bound to this workspace: run `sp open --issue ID` or pass --change")
	}
	return st.ChangeID, nil
}

func printJSON(w io.Writer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintln(w, string(b))
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ---- commands ----

func cmdLogin(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	e.resolveFlags(fs)
	email := fs.String("email", "", "account email")
	password := fs.String("password", "", "account password")
	register := fs.Bool("register", false, "register the account first")
	displayName := fs.String("name", "", "display name (register only)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *email == "" || *password == "" {
		fmt.Fprintln(stderr, "error: --email and --password are required")
		return 2
	}
	c := New(e.serverOrSession(), "")
	var (
		res LoginResult
		err error
	)
	if *register {
		res, err = c.Register(*email, *password, *displayName)
	} else {
		res, err = c.Login(*email, *password)
	}
	if err != nil {
		return e.fail(1, err)
	}
	if err := SaveSession(Session{Server: strings.TrimRight(e.serverOrSession(), "/"), Token: res.Token, Email: res.User.Email, UserID: res.User.ID}); err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, res)
		return 0
	}
	fmt.Fprintf(stdout, "logged in as %s\n", res.User.Email)
	return 0
}

// serverOrSession resolves only the server URL (no token needed for login).
func (e *env) serverOrSession() string {
	if e.server != "" {
		return e.server
	}
	if v := os.Getenv("SP_SERVER"); v != "" {
		return v
	}
	if sess, err := LoadSession(); err == nil && sess.Server != "" {
		return sess.Server
	}
	return defaultServer
}

func cmdOpen(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	e.resolveFlags(fs)
	issueID := fs.String("issue", "", "issue to open a change for")
	manual := fs.Bool("manual", false, "start a bare change without the AI split (skill flow)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *issueID == "" {
		fmt.Fprintln(stderr, "error: --issue is required")
		return 2
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}

	change, tasks, err := e.openChange(c, *issueID, *manual)
	if err != nil {
		return e.fail(1, err)
	}
	st, err := LoadState()
	if err != nil {
		return e.fail(1, err)
	}
	st.ProjectID = change.ProjectID
	st.IssueID = change.IssueID
	st.ChangeID = change.ID
	st.Phase = change.Phase
	st.Status = change.Status
	st.UpdatedAt = nowRFC3339()
	if err := SaveState(st); err != nil {
		return e.fail(1, err)
	}

	if e.json {
		printJSON(stdout, map[string]any{"change": change, "tasks": tasks})
		return 0
	}
	fmt.Fprintf(stdout, "change %s (phase %s, status %s) bound to .specpower/state.json\n",
		change.ID, change.Phase, change.Status)
	printTasks(stdout, tasks)
	return 0
}

// openChange binds an existing change for the issue or creates one; with
// manual=true a bare change is created (no AI split).
func (e *env) openChange(c *Client, issueID string, manual bool) (*Change, []TaskMapping, error) {
	change, err := c.GetChangeByIssue(issueID)
	if err == nil {
		tasks, err := c.ListTasks(change.ID)
		if err != nil {
			return change, nil, err
		}
		return change, tasks, nil
	}
	if !NotFound(err) {
		return nil, nil, err
	}
	if manual {
		change, err := c.CreateChangeManual(issueID)
		if err != nil {
			return nil, nil, err
		}
		return change, nil, nil
	}
	return c.CreateChange(issueID)
}

func printTasks(w io.Writer, tasks []TaskMapping) {
	if len(tasks) == 0 {
		return
	}
	byStage := map[int][]TaskMapping{}
	var stages []int
	for _, t := range tasks {
		if _, ok := byStage[t.Stage]; !ok {
			stages = append(stages, t.Stage)
		}
		byStage[t.Stage] = append(byStage[t.Stage], t)
	}
	for i := range stages {
		fmt.Fprintf(w, "stage %d:\n", stages[i])
		for _, t := range byStage[stages[i]] {
			fmt.Fprintf(w, "  - %s (%s)\n", t.Title, t.IssueID)
		}
	}
}

func cmdGuard(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("guard", flag.ContinueOnError)
	e.resolveFlags(fs)
	changeFlag := fs.String("change", "", "change id (default bound change)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	changeID, err := e.requireChange(*changeFlag)
	if err != nil {
		return e.fail(1, err)
	}
	report, err := c.GetGuard(changeID)
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, report)
	} else {
		fmt.Fprintf(stdout, "phase: %s -> next: %s\n", report.Phase, report.NextPhase)
		fmt.Fprintf(stdout, "phase_legal: %t\n", report.PhaseLegal)
		fmt.Fprintf(stdout, "handoff_fresh: %t\n", report.HandoffFresh)
		fmt.Fprintf(stdout, "verify_passed: %t\n", report.VerifyPassed)
		fmt.Fprintf(stdout, "can_advance: %t\n", report.CanAdvance)
		fmt.Fprintf(stdout, "can_archive: %t\n", report.CanArchive)
		for _, r := range report.Reasons {
			fmt.Fprintf(stdout, "reason: %s\n", r)
		}
	}
	if !report.CanAdvance && !report.CanArchive {
		return 1
	}
	return 0
}

func cmdHandoff(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	e.resolveFlags(fs)
	changeFlag := fs.String("change", "", "change id (default bound change)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	changeID, err := e.requireChange(*changeFlag)
	if err != nil {
		return e.fail(1, err)
	}
	change, handoff, err := c.AdvanceGuard(changeID)
	if err != nil {
		return e.fail(1, err)
	}
	if err := e.updateBoundChange(change); err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, map[string]any{"change": change, "handoff": handoff})
		return 0
	}
	fmt.Fprintf(stdout, "handoff %s: %s -> %s\n", handoff.ID, handoff.FromPhase, handoff.ToPhase)
	fmt.Fprintf(stdout, "phase is now %s\n", change.Phase)
	return 0
}

func cmdRecordCheck(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: sp state record-check <build|verify> --command CMD --exit-code N [--cwd DIR]")
		return 2
	}
	scope := args[0]
	fs := flag.NewFlagSet("state record-check", flag.ContinueOnError)
	e.resolveFlags(fs)
	changeFlag := fs.String("change", "", "change id (default bound change)")
	command := fs.String("command", "", "command that was executed")
	exitCode := fs.String("exit-code", "", "exit code of the command")
	cwd := fs.String("cwd", "", "working directory of the command (default .)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: sp state record-check <build|verify> --command CMD --exit-code N [--cwd DIR]")
		return 2
	}
	if scope != "build" && scope != "verify" {
		fmt.Fprintf(stderr, "error: invalid scope %q (want build or verify)\n", scope)
		return 2
	}
	if *command == "" {
		fmt.Fprintln(stderr, "error: --command is required")
		return 2
	}
	code, err := strconv.Atoi(*exitCode)
	if err != nil {
		fmt.Fprintln(stderr, "error: --exit-code must be an integer")
		return 2
	}
	dir := *cwd
	if dir == "" {
		dir = "."
	}

	changeID, err := e.requireChange(*changeFlag)
	if err != nil {
		return e.fail(1, err)
	}

	check := Check{
		Scope: scope, Command: *command, ExitCode: code, Cwd: dir, RecordedAt: nowRFC3339(),
	}

	if scope == "verify" {
		c, err := e.connection()
		if err != nil {
			return e.fail(1, err)
		}
		result := "fail"
		if code == 0 {
			result = "pass"
		}
		summary := fmt.Sprintf("%s exited %d", *command, code)
		content := "result: " + result + "\nsummary: " + strconv.Quote(summary) + "\n"
		if _, _, err := c.SubmitVerify(changeID, content); err != nil {
			return e.fail(1, err)
		}
	}

	st, err := LoadState()
	if err != nil {
		return e.fail(1, err)
	}
	st.Checks = append(st.Checks, check)
	st.UpdatedAt = nowRFC3339()
	if err := SaveState(st); err != nil {
		return e.fail(1, err)
	}

	if e.json {
		printJSON(stdout, check)
		return 0
	}
	if scope == "verify" {
		result := "fail"
		if code == 0 {
			result = "pass"
		}
		fmt.Fprintf(stdout, "recorded verify check: exit=%d result=%s command=%s\n", code, result, *command)
	} else {
		fmt.Fprintf(stdout, "recorded build check: exit=%d command=%s\n", code, *command)
	}
	return 0
}

func cmdVerify(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	e.resolveFlags(fs)
	changeFlag := fs.String("change", "", "change id (default bound change)")
	file := fs.String("file", "", "path to the YAML verify report")
	content := fs.String("content", "", "inline YAML verify report")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	report, err := readVerifyReport(*file, *content)
	if err != nil {
		return e.fail(2, err)
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	changeID, err := e.requireChange(*changeFlag)
	if err != nil {
		return e.fail(1, err)
	}
	result, passed, err := c.SubmitVerify(changeID, report)
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, map[string]any{"result": result, "passed": passed})
	} else {
		fmt.Fprintf(stdout, "verify report submitted: result=%s\n", result)
	}
	if !passed {
		return 1
	}
	return 0
}

// readVerifyReport resolves the report content from --file, --content or
// stdin, in that order.
func readVerifyReport(file, content string) (string, error) {
	switch {
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case content != "":
		return content, nil
	default:
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		if len(b) == 0 {
			return "", fmt.Errorf("no verify report given: use --file, --content or pipe YAML to stdin")
		}
		return string(b), nil
	}
}

func cmdArchive(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	e.resolveFlags(fs)
	changeFlag := fs.String("change", "", "change id (default bound change)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	changeID, err := e.requireChange(*changeFlag)
	if err != nil {
		return e.fail(1, err)
	}
	change, err := c.Archive(changeID)
	if err != nil {
		return e.fail(1, err)
	}
	if err := e.updateBoundChange(change); err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, change)
		return 0
	}
	fmt.Fprintf(stdout, "change %s archived\n", change.ID)
	return 0
}

// updateBoundChange refreshes the cached phase/status when the bound change
// is the one that moved.
func (e *env) updateBoundChange(change *Change) error {
	st, err := LoadState()
	if err != nil {
		return err
	}
	if st.ChangeID != change.ID {
		return nil
	}
	st.Phase = change.Phase
	st.Status = change.Status
	st.UpdatedAt = nowRFC3339()
	return SaveState(st)
}

// ---- skill commands ----

func cmdSkills(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("skills", flag.ContinueOnError)
	e.resolveFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	skills, err := c.ListSkills()
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, skills)
		return 0
	}
	for _, s := range skills {
		fmt.Fprintf(stdout, "%d. %s (%s)\n    %s\n", s.Order, s.Name, s.Key, s.Description)
	}
	return 0
}

func cmdSkill(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("skill", flag.ContinueOnError)
	e.resolveFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: sp skill <KEY>")
		return 2
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	s, err := c.GetSkill(fs.Arg(0))
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, s)
		return 0
	}
	fmt.Fprintf(stdout, "# %s (%s)\n\n%s\n", s.Name, s.Key, s.Instructions)
	return 0
}

func cmdNextSkill(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("next-skill", flag.ContinueOnError)
	e.resolveFlags(fs)
	changeFlag := fs.String("change", "", "change id (default bound change)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	changeID, err := e.requireChange(*changeFlag)
	if err != nil {
		return e.fail(1, err)
	}
	s, err := c.NextSkill(changeID)
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, s)
		return 0
	}
	fmt.Fprintf(stdout, "# %s (%s)\n\n%s\n", s.Name, s.Key, s.Instructions)
	return 0
}

// ---- skill install / list --installed / uninstall ----

// resolveSkillsDir resolves the install target: --dir wins, otherwise the
// agent default (claude-code -> ~/.claude/skills). The generic agent has no
// standard location, so it requires --dir.
func resolveSkillsDir(agent, dir string) (string, error) {
	switch agent {
	case "", "claude-code":
	case "generic":
		if dir == "" {
			return "", fmt.Errorf("--dir is required for --agent generic")
		}
	default:
		return "", fmt.Errorf("invalid agent %q (want claude-code or generic)", agent)
	}
	if dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "skills"), nil
}

// targetSkills loads the skills to install: the named keys, or the whole
// embedded registry when no key is given.
func targetSkills(keys []string) ([]*skill.Skill, error) {
	reg, err := skill.DefaultRegistry()
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return reg.List(), nil
	}
	var out []*skill.Skill
	for _, key := range keys {
		s, ok := reg.Get(key)
		if !ok {
			return nil, fmt.Errorf("unknown skill %q (run `sp skills`)", key)
		}
		out = append(out, s)
	}
	return out, nil
}

// collectPositionals parses flags and positional args in any order: the
// flag package stops at the first non-flag token, so parsing is repeated
// over the remainder while collecting positionals.
func collectPositionals(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		rest = rest[1:]
	}
	return pos, nil
}

func cmdSkillInstall(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
	e.resolveFlags(fs)
	dirFlag := fs.String("dir", "", "target skills directory (default ~/.claude/skills)")
	agent := fs.String("agent", "claude-code", "target agent type: claude-code or generic")
	force := fs.Bool("force", false, "overwrite installed skills of the same name")
	keys, err := collectPositionals(fs, args)
	if err != nil {
		return 2
	}
	dir, err := resolveSkillsDir(*agent, *dirFlag)
	if err != nil {
		return e.fail(2, err)
	}
	skills, err := targetSkills(keys)
	if err != nil {
		return e.fail(1, err)
	}
	installed, err := skill.InstallSkills(dir, skills, *force)
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, map[string]any{"installed": installed, "dir": dir})
		return 0
	}
	for _, key := range installed {
		fmt.Fprintf(stdout, "installed %s -> %s\n", key, filepath.Join(dir, key, "SKILL.md"))
	}
	return 0
}

func cmdSkillList(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("skill list", flag.ContinueOnError)
	e.resolveFlags(fs)
	installed := fs.Bool("installed", false, "list skills installed on this machine")
	dirFlag := fs.String("dir", "", "skills directory (default ~/.claude/skills)")
	agent := fs.String("agent", "claude-code", "target agent type: claude-code or generic")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*installed {
		fmt.Fprintln(stderr, "usage: sp skill list --installed [--dir PATH] [--agent claude-code|generic]")
		return 2
	}
	dir, err := resolveSkillsDir(*agent, *dirFlag)
	if err != nil {
		return e.fail(2, err)
	}
	keys, err := skill.ListInstalled(dir)
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, keys)
		return 0
	}
	if len(keys) == 0 {
		fmt.Fprintf(stdout, "no skills installed in %s\n", dir)
		return 0
	}
	for _, key := range keys {
		fmt.Fprintln(stdout, key)
	}
	return 0
}

func cmdSkillUninstall(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("skill uninstall", flag.ContinueOnError)
	e.resolveFlags(fs)
	dirFlag := fs.String("dir", "", "skills directory (default ~/.claude/skills)")
	agent := fs.String("agent", "claude-code", "target agent type: claude-code or generic")
	keys, err := collectPositionals(fs, args)
	if err != nil {
		return 2
	}
	if len(keys) == 0 {
		fmt.Fprintln(stderr, "usage: sp skill uninstall KEY... [--dir PATH] [--agent claude-code|generic]")
		return 2
	}
	dir, err := resolveSkillsDir(*agent, *dirFlag)
	if err != nil {
		return e.fail(2, err)
	}
	for _, key := range keys {
		if err := skill.UninstallSkill(dir, key); err != nil {
			return e.fail(1, err)
		}
		if !e.json {
			fmt.Fprintf(stdout, "uninstalled %s from %s\n", key, dir)
		}
	}
	if e.json {
		printJSON(stdout, map[string]any{"uninstalled": keys, "dir": dir})
	}
	return 0
}

func cmdArtifactWrite(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("artifact write", flag.ContinueOnError)
	e.resolveFlags(fs)
	changeFlag := fs.String("change", "", "change id (default bound change)")
	file := fs.String("file", "", "path to the artifact content")
	content := fs.String("content", "", "inline artifact content")
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: sp artifact write <KIND> [--file PATH | --content TEXT] [--change ID]")
		return 2
	}
	// the kind is positional and must precede the flags: the flag package
	// stops parsing at the first non-flag argument
	kind := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: sp artifact write <KIND> [--file PATH | --content TEXT] [--change ID]")
		return 2
	}
	body, err := readArtifactContent(*file, *content)
	if err != nil {
		return e.fail(2, err)
	}
	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	changeID, err := e.requireChange(*changeFlag)
	if err != nil {
		return e.fail(1, err)
	}
	a, err := c.WriteArtifact(changeID, kind, body)
	if err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, a)
		return 0
	}
	fmt.Fprintf(stdout, "wrote %s v%d to change %s\n", a.Kind, a.Version, a.ChangeID)
	return 0
}

// readArtifactContent resolves the artifact content from --file, --content
// or stdin, in that order.
func readArtifactContent(file, content string) (string, error) {
	switch {
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case content != "":
		return content, nil
	default:
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		if len(b) == 0 {
			return "", fmt.Errorf("no content given: use --file, --content or pipe text to stdin")
		}
		return string(b), nil
	}
}
