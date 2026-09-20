package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// agent is one coding assistant that can be taught to use this extension.
//
// Claude Code and Copilot read the same SKILL.md format -- frontmatter with a
// name and a description, then prose -- and differ only in where they look for
// it. That is why this is a table rather than a rendering strategy per agent.
type agent struct {
	Name string
	// GlobalDir is relative to the user's home directory.
	GlobalDir string
	// ProjectDir is relative to the working directory.
	ProjectDir string
	// Note explains anything surprising about those locations.
	Note string
}

var agents = map[string]agent{
	"claude": {
		Name:       "claude",
		GlobalDir:  ".claude/skills",
		ProjectDir: ".claude/skills",
	},
	"copilot": {
		Name:       "copilot",
		GlobalDir:  ".copilot/skills",
		ProjectDir: ".github/skills",
		Note:       "Copilot reads project skills from .github/, and user skills from ~/.copilot/",
	},
}

// skillName is the directory the skill is installed under, and the name in its
// frontmatter. It matches the command so an agent that reads the skill and an
// agent that runs `gh xeb` are talking about the same thing.
const skillName = "xeb"

func agentNames() []string {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func newHarnessCmd(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "harness",
		Short: "Teach a coding agent how to use this extension",
		Long: `harness writes a skill file describing this extension -- its commands, how it
is configured, and the rules it expects to be used under -- into the place a
coding agent looks for one.

The skill is generated from the live command tree, so it cannot drift from the
binary that wrote it: adding a subcommand and reinstalling is enough to teach
the agent about it.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}

	root.AddCommand(newHarnessInstallCmd(deps))
	return root
}

func newHarnessInstallCmd(deps Deps) *cobra.Command {
	var (
		global bool
		force  bool
		dryRun bool
		path   string
	)

	c := &cobra.Command{
		Use:   "install <agent>",
		Short: "Install the xeb skill for a coding agent",
		Long: fmt.Sprintf(`install writes a SKILL.md teaching an agent how to use this extension.

Agents: %s

Without --global the skill is installed into the current directory, so it
applies to this project only and can be committed alongside it. With --global
it goes into your home directory and applies everywhere.

  claude    <dir>/.claude/skills/xeb/SKILL.md
  copilot   ./.github/skills/xeb/SKILL.md, or ~/.copilot/skills/xeb/SKILL.md

An existing file is never overwritten without --force, and --dry-run prints
the skill and its destination without writing anything.`, strings.Join(agentNames(), ", ")),
		Example: `  gh xeb harness install claude
  gh xeb harness install copilot --global
  gh xeb harness install claude --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			target, ok := agents[strings.ToLower(args[0])]
			if !ok {
				return fmt.Errorf("unknown agent %q, want one of: %s",
					args[0], strings.Join(agentNames(), ", "))
			}

			destination, err := skillPath(deps, target, global, path)
			if err != nil {
				return err
			}

			// Rendered from this process's own command tree, so the skill
			// describes the binary that produced it.
			skill := renderSkill(NewRootCmd(deps))

			out := c.OutOrStdout()

			if dryRun {
				fmt.Fprintf(out, "Would write %s\n\n", destination)
				fmt.Fprint(out, skill)
				return nil
			}

			if _, err := os.Stat(destination); err == nil && !force {
				return fmt.Errorf("%s already exists; pass --force to replace it", destination)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("checking %s: %w", destination, err)
			}

			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", filepath.Dir(destination), err)
			}
			if err := os.WriteFile(destination, []byte(skill), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", destination, err)
			}

			fmt.Fprintf(out, "Wrote %s\n", destination)
			if target.Note != "" {
				fmt.Fprintf(out, "  %s\n", target.Note)
			}
			if !global {
				fmt.Fprintln(out, "  Project scope: commit it to share with the repository.")
			}
			return nil
		},
	}

	c.Flags().BoolVar(&global, "global", false, "Install for every project, in your home directory")
	c.Flags().BoolVar(&force, "force", false, "Replace an existing skill file")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Print the skill and its destination without writing")
	c.Flags().StringVar(&path, "path", "", "Write here instead of the agent's conventional location")

	return c
}

// skillPath decides where the skill goes. An explicit --path wins; otherwise
// the agent's table entry is joined to the home or working directory.
func skillPath(deps Deps, target agent, global bool, override string) (string, error) {
	if override != "" {
		// A directory means "put the skill inside"; anything else is taken as
		// the file itself, so --path can name an exact destination.
		if info, err := os.Stat(override); err == nil && info.IsDir() {
			return filepath.Join(override, skillName, "SKILL.md"), nil
		}
		return override, nil
	}

	var (
		base string
		dir  string
		err  error
	)
	if global {
		base, err = deps.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding your home directory: %w", err)
		}
		dir = target.GlobalDir
	} else {
		base, err = deps.WorkingDir()
		if err != nil {
			return "", fmt.Errorf("finding the current directory: %w", err)
		}
		dir = target.ProjectDir
	}

	return filepath.Join(base, filepath.FromSlash(dir), skillName, "SKILL.md"), nil
}

// renderSkill builds the skill document from the command tree it is given.
//
// The command list is walked rather than written out, so a new subcommand is
// documented for the agent the moment it is registered. Everything an agent
// cannot infer from `--help` -- the safety rules, which commands mutate -- is
// prose below it.
func renderSkill(root *cobra.Command) string {
	var b strings.Builder

	fmt.Fprintf(&b, `---
name: %s
description: |
  Query GitHub, HashiCorp Vault and a Backstage software catalog through the
  gh CLI extension %q. Use for: what a Vault token can reach, reading KV
  secrets, listing catalog entities and their owners, finding the catalog
  entry for the current repository, listing scaffolder templates and the
  inputs they take, and running one.
---

# %s

`, skillName, "gh "+skillName, skillName)

	fmt.Fprintf(&b, "A GitHub CLI extension. Invoke it as `gh %s <command>`. Generated from %s.\n\n",
		skillName, resolveVersion())

	b.WriteString("## Commands\n\n")
	writeCommandTree(&b, root, "")

	b.WriteString(`
Every command accepts ` + "`--json`" + `. Prefer it when you need to act on the result;
the text output is aligned for a human and its columns are not a contract.

## Configuration

| | |
| --- | --- |
| GitHub | Your ` + "`gh`" + ` credentials. Nothing extra to set. |
| Vault | The standard ` + "`VAULT_*`" + ` variables, exactly as the ` + "`vault`" + ` CLI reads them. Falling back to a login through Vault's GitHub auth method with your ` + "`gh`" + ` token. |
| Backstage | ` + "`BACKSTAGE_BASE_URL`" + ` and ` + "`BACKSTAGE_TOKEN`" + `, or ` + "`--vault-secret <path>`" + ` to read both from a Vault KV secret with ` + "`url`" + ` and ` + "`token`" + ` fields. |

## Rules

- **Never pass a token as a command-line argument.** There is no ` + "`--token`" + `
  flag anywhere in this tree, deliberately: an argument is visible in shell
  history and in ` + "`ps`" + ` to every other user on the machine. Use the
  environment variable, or ` + "`--vault-secret`" + `.
- **` + "`backstage create`" + ` is the only command that changes anything outside the
  machine.** A successful run creates a GitHub repository and registers it in
  the catalog, and interrupting the command undoes none of it. Run it with
  ` + "`--dry-run`" + ` first; that validates every value against the template's
  schema and prints what would be sent, without sending it.
- **Secret values are masked by default.** ` + "`vault get`" + ` shows which fields
  exist and how long each value is. Use ` + "`--reveal`" + ` only when the human
  asked to see the value, and ` + "`--field <key>`" + ` when piping one value
  somewhere.
- **` + "`vault`" + ` never prints the token itself**, and neither should you.

## Worked examples

` + "```sh" + `
# What can this Vault token actually reach?
gh ` + skillName + ` vault
gh ` + skillName + ` vault can secret/data/prod/db

# Which fields does a secret have, without printing them?
gh ` + skillName + ` vault get secret/prod/db

# Who owns the services in the catalog?
gh ` + skillName + ` backstage entities --kind component --json

# Is the repository I am in catalogued?
gh ` + skillName + ` backstage repo

# What can the portal create, and what does it need?
gh ` + skillName + ` backstage ofertas --json

# Run a template, checking first
gh ` + skillName + ` backstage create terraform-module --field name=s3-bucket --dry-run
` + "```" + `

## Failure modes worth recognising

- ` + "`no Vault token available`" + ` — set ` + "`VAULT_TOKEN`" + `, run ` + "`vault login`" + `,
  or run ` + "`gh auth login`" + ` so the extension can log in through GitHub.
- ` + "`401 AuthenticationError`" + ` from the catalog — ` + "`BACKSTAGE_TOKEN`" + ` is
  missing or expired.
- ` + "`no entity ... in the catalog`" + ` — the reference is
  ` + "`[<kind>:][<namespace>/]<name>`" + `, and the kind is required.
- A ` + "`repoUrl`" + ` template parameter is not a URL. It is
  ` + "`github.com?owner=acme&repo=payments`" + `.
`)

	return b.String()
}

// writeCommandTree lists a command's children, and their children, skipping
// cobra's own generated commands -- an agent does not need `completion`.
func writeCommandTree(b *strings.Builder, parent *cobra.Command, prefix string) {
	children := parent.Commands()
	sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })

	for _, child := range children {
		if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		fmt.Fprintf(b, "- `gh %s %s%s` — %s\n", skillName, prefix, child.Name(), child.Short)
		writeCommandTree(b, child, prefix+child.Name()+" ")
	}
}
