package profile

// Agent describes a supported coding agent and the environment variables
// that redirect it into a profile.
type Agent struct {
	// Name is the agent's executable and profile subdirectory name.
	Name string
	// HomeVariable is the environment variable the agent reads to locate
	// its configuration home.
	HomeVariable string
	// ClearHomeVariable marks agents whose home variable must be removed
	// from a profiled environment instead of set. Claude Code re-keys its
	// macOS keychain service with a hash of CLAUDE_CONFIG_DIR, so setting
	// it would give every profile a separate OAuth login instead of the
	// shared one; the composed home's ~/.claude alias already routes the
	// default location into the profile.
	ClearHomeVariable bool
	// ProxyVariable is the environment variable the agent's client reads
	// for an alternative API endpoint.
	ProxyVariable string
}

// Agents lists the supported agents in presentation order.
var Agents = []Agent{
	{Name: "codex", HomeVariable: "CODEX_HOME", ProxyVariable: "OPENAI_BASE_URL"},
	{Name: "claude", HomeVariable: "CLAUDE_CONFIG_DIR", ClearHomeVariable: true, ProxyVariable: "ANTHROPIC_BASE_URL"},
}

// HomeEnvironment adjusts environment so the agent locates its configuration
// home inside the profile: either by pointing the home variable at home or,
// for agents whose home variable must stay unset, by clearing an inherited
// value so the composed private home's aliases take over.
func (a Agent) HomeEnvironment(environment []string, home string) []string {
	if a.ClearHomeVariable {
		return RemoveEnvironment(environment, a.HomeVariable)
	}
	return ReplaceEnvironment(environment, a.HomeVariable, home)
}

// AgentByName looks up a supported agent.
func AgentByName(name string) (Agent, bool) {
	for _, agent := range Agents {
		if agent.Name == name {
			return agent, true
		}
	}
	return Agent{}, false
}
