package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// harnessDeps points both roots at temp directories, so an install in a test
// can never touch the developer's real home or working directory.
func harnessDeps(t *testing.T) (Deps, string, string) {
	t.Helper()

	home, work := t.TempDir(), t.TempDir()
	deps := testDeps()
	deps.UserHomeDir = func() (string, error) { return home, nil }
	deps.WorkingDir = func() (string, error) { return work, nil }
	return deps, home, work
}

func TestHarnessInstallProjectScope(t *testing.T) {
	cases := []struct {
		agent string
		want  string
	}{
		{"claude", ".claude/skills/xeb/SKILL.md"},
		{"copilot", ".github/skills/xeb/SKILL.md"},
	}

	for _, tc := range cases {
		t.Run(tc.agent, func(t *testing.T) {
			deps, home, work := harnessDeps(t)

			out, err := runRoot(t, deps, "harness", "install", tc.agent)
			if err != nil {
				t.Fatalf("harness install %s: %v", tc.agent, err)
			}

			path := filepath.Join(work, filepath.FromSlash(tc.want))
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading the installed skill: %v", err)
			}
			if !strings.HasPrefix(string(body), "---\nname: xeb\n") {
				t.Errorf("the skill has no frontmatter:\n%s", firstLines(string(body), 3))
			}
			if !strings.Contains(out, path) {
				t.Errorf("output does not say where it wrote:\n%s", out)
			}
			// Project scope must not touch the home directory.
			if entries, _ := os.ReadDir(home); len(entries) != 0 {
				t.Errorf("a project install wrote into the home directory: %v", entries)
			}
		})
	}
}

func TestHarnessInstallGlobalScope(t *testing.T) {
	cases := []struct {
		agent string
		want  string
	}{
		{"claude", ".claude/skills/xeb/SKILL.md"},
		{"copilot", ".copilot/skills/xeb/SKILL.md"},
	}

	for _, tc := range cases {
		t.Run(tc.agent, func(t *testing.T) {
			deps, home, work := harnessDeps(t)

			if _, err := runRoot(t, deps, "harness", "install", tc.agent, "--global"); err != nil {
				t.Fatalf("harness install %s --global: %v", tc.agent, err)
			}

			if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(tc.want))); err != nil {
				t.Fatalf("the global skill was not written: %v", err)
			}
			if entries, _ := os.ReadDir(work); len(entries) != 0 {
				t.Errorf("a global install wrote into the working directory: %v", entries)
			}
		})
	}
}

func TestHarnessInstallSaysWhereCopilotLooks(t *testing.T) {
	deps, _, _ := harnessDeps(t)

	out, err := runRoot(t, deps, "harness", "install", "copilot", "--global")
	if err != nil {
		t.Fatalf("harness install: %v", err)
	}
	// The two Copilot roots differ from each other, which is worth saying.
	if !strings.Contains(out, ".github/") || !strings.Contains(out, "~/.copilot/") {
		t.Errorf("output does not explain the Copilot locations:\n%s", out)
	}
}

func TestHarnessInstallRefusesToOverwrite(t *testing.T) {
	deps, _, work := harnessDeps(t)
	path := filepath.Join(work, ".claude", "skills", "xeb", "SKILL.md")

	if _, err := runRoot(t, deps, "harness", "install", "claude"); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := os.WriteFile(path, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatalf("editing the skill: %v", err)
	}

	_, err := runRoot(t, deps, "harness", "install", "claude")
	if err == nil {
		t.Fatal("the second install overwrote an existing skill")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error does not say how to proceed: %v", err)
	}

	body, _ := os.ReadFile(path)
	if string(body) != "hand-edited\n" {
		t.Error("the file was modified despite the refusal")
	}

	if _, err := runRoot(t, deps, "harness", "install", "claude", "--force"); err != nil {
		t.Fatalf("--force install: %v", err)
	}
	body, _ = os.ReadFile(path)
	if string(body) == "hand-edited\n" {
		t.Error("--force did not replace the file")
	}
}

func TestHarnessInstallDryRunWritesNothing(t *testing.T) {
	deps, _, work := harnessDeps(t)

	out, err := runRoot(t, deps, "harness", "install", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("harness install --dry-run: %v", err)
	}
	if !strings.Contains(out, "Would write") || !strings.Contains(out, "name: xeb") {
		t.Errorf("dry run does not show the destination and the skill:\n%s", out)
	}
	if entries, _ := os.ReadDir(work); len(entries) != 0 {
		t.Errorf("--dry-run wrote something: %v", entries)
	}
}

func TestHarnessInstallUnknownAgent(t *testing.T) {
	deps, _, work := harnessDeps(t)

	_, err := runRoot(t, deps, "harness", "install", "cursor")
	if err == nil {
		t.Fatal("want an error for an unknown agent")
	}
	for _, want := range []string{"cursor", "claude", "copilot"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
	if entries, _ := os.ReadDir(work); len(entries) != 0 {
		t.Errorf("something was written for an unknown agent: %v", entries)
	}
}

func TestHarnessInstallAgentNameIsCaseInsensitive(t *testing.T) {
	deps, _, work := harnessDeps(t)

	if _, err := runRoot(t, deps, "harness", "install", "Claude"); err != nil {
		t.Fatalf("harness install Claude: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, ".claude", "skills", "xeb", "SKILL.md")); err != nil {
		t.Fatalf("nothing was installed: %v", err)
	}
}

func TestHarnessInstallPathOverride(t *testing.T) {
	deps, home, work := harnessDeps(t)
	elsewhere := t.TempDir()

	// A directory means "put the skill inside it".
	if _, err := runRoot(t, deps, "harness", "install", "claude", "--path", elsewhere); err != nil {
		t.Fatalf("harness install --path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "xeb", "SKILL.md")); err != nil {
		t.Fatalf("the skill was not written under the given directory: %v", err)
	}

	// A non-directory is taken as the file itself.
	exact := filepath.Join(elsewhere, "custom.md")
	if _, err := runRoot(t, deps, "harness", "install", "claude", "--path", exact); err != nil {
		t.Fatalf("harness install --path <file>: %v", err)
	}
	if _, err := os.Stat(exact); err != nil {
		t.Fatalf("the skill was not written to the named file: %v", err)
	}

	for _, dir := range []string{home, work} {
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("--path still wrote into %s: %v", dir, entries)
		}
	}
}

func TestHarnessInstallReportsAnUnresolvableHome(t *testing.T) {
	deps, _, _ := harnessDeps(t)
	deps.UserHomeDir = func() (string, error) { return "", errors.New("no home") }

	_, err := runRoot(t, deps, "harness", "install", "claude", "--global")
	if err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestSkillDescribesTheLiveCommandTree(t *testing.T) {
	skill := renderSkill(NewRootCmd(testDeps()))

	// Every registered command, including nested ones, must appear -- that is
	// the whole point of generating rather than writing this by hand.
	for _, want := range []string{
		"`gh xeb vault`", "`gh xeb vault can`", "`gh xeb vault get`",
		"`gh xeb backstage entities`", "`gh xeb backstage ofertas`",
		"`gh xeb backstage create`", "`gh xeb doctor`", "`gh xeb whoami`",
		"`gh xeb harness install`",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("the skill does not mention %s", want)
		}
	}

	// Cobra's own commands are noise for an agent.
	for _, unwanted := range []string{"`gh xeb completion`", "`gh xeb help`"} {
		if strings.Contains(skill, unwanted) {
			t.Errorf("the skill lists %s", unwanted)
		}
	}

	// The rules are the part an agent cannot infer from --help.
	for _, want := range []string{
		"Never pass a token as a command-line argument",
		"--dry-run", "masked by default", "repoUrl",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("the skill is missing the guidance %q", want)
		}
	}
}

func TestSkillCoversEveryCommandThatExists(t *testing.T) {
	skill := renderSkill(NewRootCmd(testDeps()))

	var missing []string
	var visit func(prefix string, children []*cobra.Command)
	visit = func(prefix string, children []*cobra.Command) {
		for _, child := range children {
			if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
				continue
			}
			token := "`gh xeb " + prefix + child.Name() + "`"
			if !strings.Contains(skill, token) {
				missing = append(missing, token)
			}
			visit(prefix+child.Name()+" ", child.Commands())
		}
	}
	visit("", NewRootCmd(testDeps()).Commands())

	// Adding a subcommand and forgetting the skill is impossible by
	// construction; this fails if the generator ever stops walking.
	if len(missing) > 0 {
		t.Errorf("the skill omits: %s", strings.Join(missing, ", "))
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
