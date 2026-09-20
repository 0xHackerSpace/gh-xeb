package cmd

import (
	"fmt"
	"strings"
)

// vaultEnvironments are the deployment environments a secret path carries.
// The convention this follows is secret/<team>/<env>/<app>, which is what the
// policies in this organisation are written against.
var vaultEnvironments = []string{"dev", "uat", "prod"}

const (
	// defaultEnv is what --env resolves to when it is not given.
	defaultEnv = "dev"
	// envPlaceholder is the explicit form: write it into a path and --env
	// fills it in, at whatever position you put it.
	envPlaceholder = "{env}"
)

// validateEnv rejects an environment the convention does not have, naming the
// ones it does. A typo here would otherwise read a path that quietly does not
// exist, or -- worse with a prefix policy -- one that does.
func validateEnv(env string) error {
	for _, known := range vaultEnvironments {
		if env == known {
			return nil
		}
	}
	return fmt.Errorf("unknown environment %q, want one of: %s",
		env, strings.Join(vaultEnvironments, ", "))
}

func isEnvironment(segment string) bool {
	for _, known := range vaultEnvironments {
		if segment == known {
			return true
		}
	}
	return false
}

// applyEnv resolves the environment in a Vault path.
//
// Two forms, and the difference between them is the whole design:
//
//   - A {env} placeholder is always substituted. It is explicit, it works at
//     any position, and it is the form to reach for.
//   - A literal segment that already names an environment is rewritten only
//     when --env was actually given. Without the flag, a path is passed
//     through untouched, so `vault get secret/abcd/dev/api` keeps meaning
//     exactly what it says and the flag's default cannot silently rewrite
//     anyone's path.
//
// explicit is cobra's Changed(), not "the value differs from the default":
// `--env dev` against secret/abcd/prod/api is a deliberate downgrade and does
// rewrite, which is the point of typing it.
func applyEnv(path, env string, explicit bool) string {
	if strings.Contains(path, envPlaceholder) {
		return strings.ReplaceAll(path, envPlaceholder, env)
	}
	if !explicit {
		return path
	}

	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if isEnvironment(segment) {
			segments[i] = env
		}
	}
	return strings.Join(segments, "/")
}

// envFlagUsage keeps the flag's help identical wherever it is registered.
func envFlagUsage() string {
	return fmt.Sprintf("Environment segment of a Vault path (%s); fills in {env}, "+
		"and rewrites an existing environment segment when given explicitly",
		strings.Join(vaultEnvironments, "|"))
}
