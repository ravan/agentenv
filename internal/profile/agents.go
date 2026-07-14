package profile

// Agent describes a supported coding agent and the environment variables
// that redirect it into a profile.
type Agent struct {
	// Name is the agent's executable and profile subdirectory name.
	Name string
	// HomeVariable is the environment variable the agent reads to locate
	// its configuration home.
	HomeVariable string
	// ProxyVariable is the environment variable the agent's client reads
	// for an alternative API endpoint.
	ProxyVariable string
}

// Agents lists the supported agents in presentation order.
var Agents = []Agent{
	{Name: "codex", HomeVariable: "CODEX_HOME", ProxyVariable: "OPENAI_BASE_URL"},
	{Name: "claude", HomeVariable: "CLAUDE_CONFIG_DIR", ProxyVariable: "ANTHROPIC_BASE_URL"},
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
