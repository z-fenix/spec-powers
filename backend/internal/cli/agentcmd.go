package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// cmdAgentRegister registers a local-runtime agent on the server (creating
// the agent record) and saves its runtime credential under ~/.sp/agents/.
func cmdAgentRegister(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("agent register", flag.ContinueOnError)
	e.resolveFlags(fs)
	name := fs.String("name", "", "agent name (also the local credential name)")
	description := fs.String("description", "", "what this agent does")
	force := fs.Bool("force", false, "re-register over an existing local credential of the same name")
	skills, err := collectPositionals(fs, args)
	if err != nil {
		return 2
	}
	if *name == "" {
		fmt.Fprintln(stderr, "error: --name is required")
		return 2
	}
	if _, err := LoadAgentCredential(*name); err == nil && !*force {
		fmt.Fprintf(stderr, "error: agent %q already registered locally (pass --force to re-register)\n", *name)
		return 1
	}

	c, err := e.connection()
	if err != nil {
		return e.fail(1, err)
	}
	server := strings.TrimRight(e.serverOrSession(), "/")
	res, err := c.RegisterAgent(*name, *description, skills)
	if err != nil {
		return e.fail(1, err)
	}
	cred := AgentCredential{
		Server:    server,
		AgentID:   res.Agent.ID,
		AgentName: res.Agent.Name,
		Token:     res.Token,
	}
	if err := SaveAgentCredential(*name, cred); err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, map[string]any{"agent": res.Agent, "credential": cred})
		return 0
	}
	fmt.Fprintf(stdout, "registered agent %s (%s) as a local runtime\n", res.Agent.Name, res.Agent.ID)
	fmt.Fprintf(stdout, "credential saved; start it with: sp agent run --name %s\n", *name)
	return 0
}

// cmdAgentDeregister deletes the agent on the server using its own runtime
// credential (revoking it everywhere) and removes the local credential.
func cmdAgentDeregister(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("agent deregister", flag.ContinueOnError)
	e.resolveFlags(fs)
	name := fs.String("name", "", "registered agent name (default the only registered agent)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cred, err := resolveAgentCredential(*name)
	if err != nil {
		return e.fail(1, err)
	}
	c := New(cred.Server, cred.Token)
	if err := c.DeleteAgent(cred.AgentID); err != nil && !NotFound(err) {
		return e.fail(1, err)
	}
	if err := DeleteAgentCredential(cred.AgentName); err != nil {
		return e.fail(1, err)
	}
	if e.json {
		printJSON(stdout, map[string]any{"deregistered": cred.AgentName, "agent_id": cred.AgentID})
		return 0
	}
	fmt.Fprintf(stdout, "deregistered agent %s (%s); its runtime credential is revoked\n", cred.AgentName, cred.AgentID)
	return 0
}
