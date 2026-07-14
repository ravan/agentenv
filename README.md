# agentenv

`agentenv` keeps separate user-level environments for Codex and Claude Code and
selects one per project. The globally installed `codex` and `claude` binaries
stay untouched. Each profile has its own filesystem-backed configuration,
skills, plugins, and instruction files, while OAuth authentication is shared
between profiles.

## Install

Go 1.24 or newer is required to build from source.

From a local checkout, run:

```sh
cd /path/to/agentenv
go install ./cmd/agentenv
```

Go installs the executable into `GOBIN`, or into `$(go env GOPATH)/bin` when
`GOBIN` is unset. Make sure that directory is on `PATH` before running
`agentenv`.

After the repository is published as a Go module, it can also be installed by
module path:

```sh
go install github.com/ravan/agentenv/cmd/agentenv@latest
```

Codex and/or Claude Code must already be installed and available on `PATH`.

## Quick start

Create a `default` profile and any specialized profiles you want. Each starts
with private agent state, while OAuth remains shared between them:

```sh
agentenv new default
agentenv new superpowers
agentenv list
```

From a project directory, select a profile and launch an agent:

```sh
agentenv use superpowers
agentenv current
agentenv codex
agentenv claude --permission-mode plan
```

`activate` is an alias for `use`. The `.agentenv` file contains only the profile
name. Keep it in the repository when the selection is shared by the project, or
add it to the project's ignore file when it is personal. A selection affects
future launches only: an already-running agent keeps the profile it started
with. Restart agents after changing profiles.

Verify the environment from a newly launched profiled session. Each agent has a
matching configuration variable, and both share the composed `HOME`:

```sh
printf '%s\n' "$CODEX_HOME"          # agentenv codex
printf '%s\n' "$CLAUDE_CONFIG_DIR"   # agentenv claude
printf '%s\n' "$HOME"                # both
```

`CODEX_HOME` must name `<profile-root>/<profile>/codex`, `CLAUDE_CONFIG_DIR`
must name `<profile-root>/<profile>/claude`, and `HOME` must name
`<profile-root>/<profile>/home`. If a relevant variable is empty or points to
the real home, exit immediately and correct the launcher.

Use the default profile whenever you want a clean baseline:

```sh
agentenv use default
```

Global agent settings and onboarding state are not copied into profiles.
Install and configure filesystem-backed skills or plugins from a wrapped agent
session; they are then written beneath that profile's Codex, Claude, or
`.agents` directory. Plugins or skills already present in the real `~/.codex`,
`~/.claude`, or `~/.agents` are not imported. Reinstall the ones wanted in each
profile. Codex account-backed remote plugins are the exception described below.

Delete a profile when it is no longer needed:

```sh
agentenv delete security-review
```

Deletion removes the profile's complete Codex, Claude, `.agents`, and composed
home trees. It does not remove the shared OAuth store.

## How it works

When an agent is launched, `agentenv` searches the current directory and its
parents for a `.agentenv` selection, then sets the matching configuration root
and a composed profile home:

| Command | Environment variable | Profile directory |
| --- | --- | --- |
| `agentenv codex` | `CODEX_HOME` | `<profile-root>/<profile>/codex` |
| `agentenv claude` | `CLAUDE_CONFIG_DIR` | `<profile-root>/<profile>/claude` |

Both commands set `HOME` and `USERPROFILE` to
`<profile-root>/<profile>/home`. `HOMEDRIVE`, `HOMEPATH`, the XDG config, cache,
data, and state roots, and Windows `APPDATA` and `LOCALAPPDATA` are redirected
under that private home as well.

Arguments, standard streams, working directory, and the agent's exit code are
preserved. Project-level agent configuration remains part of the project and is
still discovered normally; profiles isolate filesystem-backed user
configuration, skills, plugins, and instruction files while OAuth
authentication is shared.

> [!IMPORTANT]
> Codex remote plugins installed from the interactive `/plugins` screen are
> stored by OpenAI against the signed-in account. Codex deliberately loads that
> account state into every `CODEX_HOME` using the same OAuth identity.
> Environment-variable and filesystem isolation cannot make those remote
> installations profile-specific. See [Codex remote plugins](#codex-remote-plugins).

## Strict laptop setup

Profile isolation applies only to processes launched by `agentenv`. The
`agentenv use` command writes the project selection; it cannot change the
environment of a running agent or a later process launched directly as `codex`
or `claude`.

For strict isolation, route every new shell launch through `agentenv`. Add these
functions to `~/.zshrc` after the code that configures `PATH`:

```zsh
codex() {
  agentenv codex "$@"
}

claude() {
  agentenv claude "$@"
}
```

If Codex is wrapped by another program, that wrapper must also run inside the
profile. For example, a Headroom function can use `agentenv run`:

```zsh
codex-headroom() {
  HEADROOM_TELEMETRY=off \
    agentenv run codex -- \
    headroom wrap codex --no-serena --no-proxy --yolo "$@"
}
```

Reload the shell after editing it:

```sh
exec zsh
```

### Reject accidental global Codex launches

Codex supports `SessionStart` hooks. A global hook can stop a session when its
working directory selects an agentenv profile but its `CODEX_HOME` or `HOME`
does not match that profile.

First find the absolute executable path:

```sh
command -v agentenv
```

Then add this entry to `~/.codex/hooks.json`, replacing
`/absolute/path/to/agentenv` with that path. If the file already contains hooks,
merge this `SessionStart` entry instead of replacing them.

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "/absolute/path/to/agentenv guard codex",
            "statusMessage": "Checking agentenv profile isolation"
          }
        ]
      }
    ]
  }
}
```

Review and trust the hook when Codex first reports it. The guard allows global
Codex sessions in directories without an `.agentenv` selection, but fails
closed when a selected project is launched with the wrong profile environment.

### Reject accidental global Claude launches

Claude Code supports `SessionStart` hooks. The same guard stops a session when
its working directory selects an agentenv profile but its `CLAUDE_CONFIG_DIR`
or `HOME` does not match that profile.

First find the absolute executable path:

```sh
command -v agentenv
```

Then add this entry to the real `~/.claude/settings.json`, replacing
`/absolute/path/to/agentenv` with that path. If the file already contains hooks,
merge this `SessionStart` entry instead of replacing them.

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "/absolute/path/to/agentenv guard claude"
          }
        ]
      }
    ]
  }
}
```

The guard allows global Claude sessions in directories without an `.agentenv`
selection, but fails closed when a selected project is launched with the wrong
profile environment. As with Codex, configure Claude Code launchers — IDEs,
desktop applications, terminal managers, and automation — as `agentenv claude`,
or launch their wrapper with `agentenv run claude -- ...`. A launcher that
executes an absolute Claude binary bypasses agentenv and cannot provide profile
isolation.

### cmux

cmux currently records and resumes Codex by its absolute binary path. That
bypasses shell functions and `agentenv`. Disable **Resume Agent Sessions on
Reopen** in cmux settings, or set this in `~/.config/cmux/settings.json`:

```json
{
  "terminal": {
    "autoResumeAgentSessions": false
  }
}
```

Do not resume an old Codex panel after selecting another profile. Close it and
start a new `codex` or `codex-headroom` command so the shell function runs.
The startup guard rejects an accidentally resumed global session in a selected
project.

The same rule applies to IDEs, desktop applications, terminal managers, and
automation: configure their Codex command as `agentenv codex`, or launch their
wrapper with `agentenv run codex -- ...`. A launcher that executes an absolute
Codex binary bypasses agentenv and cannot provide profile isolation.

## Codex remote plugins

Codex has two materially different kinds of plugin state:

- Local and filesystem-backed marketplace plugins are stored below
  `CODEX_HOME`. They are isolated by agentenv profiles.
- Remote curated plugins installed through the interactive `/plugins` screen
  are stored by OpenAI against the authenticated account. They are shared by
  every profile using that OAuth account, even when `CODEX_HOME`, `HOME`, XDG,
  and AppData paths are all private.

This behavior comes from Codex itself. In Codex `v0.144.4`, app-server's
`plugin/installed` request fetches `/ps/plugins/installed` with the shared
account credentials and synchronizes the returned bundles into the active
`CODEX_HOME`. Setting `remote_plugin = false` hides remote discovery but does
not suppress installed-account synchronization.

Consequently, the following workflow is account-wide and cannot provide
profile-specific installation while OAuth is shared:

```text
/plugins -> OpenAI Curated -> Install
```

Treat the two mechanisms as complementary features:

- Install through the interactive `/plugins` screen when the plugin should be
  shared by every profile on the account.
- Install from a local or Git-backed marketplace when the plugin must exist in
  only the selected profile.

The `codex-plugin` command runs Codex's non-interactive plugin CLI inside the
selected profile, so profile-local installations never require entering an
agent session:

```sh
agentenv codex-plugin marketplace add https://github.com/obra/superpowers
agentenv codex-plugin add superpowers@superpowers-dev
agentenv codex-plugin list
agentenv codex-plugin remove superpowers@superpowers-dev
```

For a plugin available both remotely and from a Git marketplace, pick one
mechanism. When the remote installation already exists on the account,
uninstall it from `/plugins` before installing the Git-backed variant in a
profile.

There are only three ways to make account-backed remote selections differ:
use separate authenticated accounts, change/intercept Codex's account-service
traffic, or have Codex add a profile-scoped remote-install feature. agentenv
remains a process/environment wrapper and does not run a network proxy.

## Helper tool integrations

Token-saving helpers such as [rtk](https://github.com/rtk-ai/rtk) and
[tokensave](https://github.com/aovestdipaperino/tokensave) install themselves
by editing the global agent configuration: instruction blocks in
`~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`, hooks in
`~/.claude/settings.json`, and MCP server entries in `~/.claude.json` and
`~/.codex/config.toml`. Both tools resolve those paths through `HOME`, so
running their own installers inside a profile environment writes to the
profile instead of the real global configuration.

`enable` and `disable` wrap exactly that. They resolve the active profile,
swap in the profile environment (`HOME`, `CODEX_HOME`, `CLAUDE_CONFIG_DIR`),
and run the tool's native installer or uninstaller for both the Claude and
Codex homes:

```sh
agentenv use superpowers
agentenv enable rtk         # rtk init -g --auto-patch / rtk init -g --codex
agentenv enable tokensave   # tokensave install --agent claude|codex
agentenv disable rtk
agentenv disable tokensave
```

Each profile decides independently which helpers are active. The tool
binaries themselves stay installed on the real system; enable/disable only
adds or removes their agent wiring in the selected profile. Integrations
already present in the real `~/.claude` or `~/.codex` are not imported into
profiles — enable them per profile.

## Agent proxy URLs

Each profile can route an agent's API traffic through a proxy or LLM gateway
(for example LiteLLM). The URL is stored in the profile's `config.json` and
exported as the agent's endpoint variable on every profiled launch:

| Agent | Environment variable |
| --- | --- |
| `codex` | `OPENAI_BASE_URL` |
| `claude` | `ANTHROPIC_BASE_URL` |

```sh
agentenv use gateway
agentenv proxy set codex http://localhost:4000/openai
agentenv proxy set claude http://localhost:4000/anthropic
agentenv proxy show
agentenv proxy unset codex
```

An absolute `http` or `https` URL is required. The setting applies to
`agentenv codex`, `agentenv claude`, `agentenv run`, and `agentenv
codex-plugin` launches in that profile. When no proxy is configured, any
endpoint variable already present in the shell is inherited unchanged, so a
profile without a proxy keeps the default provider endpoint.

## Storage

Profiles live under `~/.agent-profiles` by default:

```text
~/.agent-profiles/
├── shared/
│   ├── codex-auth.json
│   └── claude-credentials.json
├── default/
│   ├── config.json
│   ├── codex/
│   ├── claude/
│   └── home/
│       ├── .codex -> ../codex
│       ├── .claude -> ../claude
│       ├── .claude.json -> ../claude/.claude.json
│       └── .agents/
└── superpowers/
    ├── codex/
    ├── claude/
    └── home/
```

Set `AGENTENV_HOME` to use another profile root:

```sh
AGENTENV_HOME="$HOME/.config/agentenv/profiles" agentenv list
```

Each home is hermetic and owns its `.codex`, `.claude`, `.claude.json`, and
`.agents` entries. Ordinary real-home files and directories such as
`.gitconfig`, `.ssh`, and `.config` are not visible through the profile home.
Agent-created XDG and AppData state also stays beneath the selected profile.

Existing profiles are upgraded lazily before their next launch. The upgrade
removes only top-level profile-home symlinks that resolve to the same-named
entry in the real home; it never removes their targets. Profile-owned files and
unrelated links are preserved. Reserved agent aliases are recreated when safe.
If a reserved entry conflicts with the required private layout, `agentenv`
fails before starting the agent.

This is filesystem-state isolation, not an operating-system sandbox.
Repository-local skills, plugins, and instructions remain shared because the
repository is outside the profile home. Unrelated environment credentials,
sockets, `PATH`, runtime directories, and temporary directories remain
inherited. OAuth remains shared as described below.

Direct `~/.codex`, `~/.claude`, and misplaced plugin data are left untouched.
`agentenv` does not move Codex-managed plugin caches between profiles or rewrite
an agent's private plugin metadata.

## Shared OAuth

When Codex or Claude uses the operating system credential store, OAuth is
already shared independently of the selected configuration directory. For
file-based credential storage, `agentenv` links these agent-specific paths to
the shared store:

```text
<profile>/codex/auth.json          -> ../../shared/codex-auth.json
<profile>/claude/.credentials.json -> ../../shared/claude-credentials.json
```

Sign in from any wrapped profile once; other profiles then use the same OAuth
session. Some agent versions save credentials by atomically replacing the
credential file. After an agent exits, `agentenv` moves the refreshed data back
to the shared store and restores the link.

When the shared store is first created, existing file-based credentials are
copied from `~/.codex/auth.json` and `~/.claude/.credentials.json`. Existing
direct Codex and Claude installations therefore remain signed in, and their
credential files are left in place. Agent launches repeat this check so profiles
created by an earlier `agentenv` release are upgraded automatically.

The shared files contain refresh and access tokens. They are created with
owner-only permissions; do not commit, copy into a project, or expose them in
logs.

## Commands

```text
agentenv new <name>        create an isolated composed home with shared OAuth
agentenv list              list available profiles
agentenv delete <name>     permanently remove a profile
agentenv use <name>        select a profile in the current directory
agentenv activate <name>   alias for use
agentenv current           print the selection found from the current context
agentenv run <agent> -- <command> [args...]
                           run a wrapper inside the selected agent profile
agentenv codex-plugin <command> [args...]
                           manage profile-local Codex plugins (wraps 'codex plugin')
agentenv enable <rtk|tokensave>
                           install a helper tool integration into the profile
agentenv disable <rtk|tokensave>
                           remove a helper tool integration from the profile
agentenv proxy set <codex|claude> <url>
                           route the agent's API traffic through a proxy URL
agentenv proxy unset <codex|claude>
                           remove the agent's proxy URL from the profile
agentenv proxy show        print the proxy URLs configured for the profile
agentenv codex [args...]   launch Codex with the selected profile
agentenv claude [args...]  launch Claude Code with the selected profile
```
