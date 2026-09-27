# GitHub Authentication Configuration
# This file configures Vault to authenticate users via GitHub

# Map specific GitHub teams to Vault policies
resource "vault_github_team" "admins" {
  backend  = vault_github_auth_backend.github.id
  team     = "admins"
  policies = ["admin"]

  depends_on = [vault_github_auth_backend.github]
}

resource "vault_github_team" "developers" {
  backend  = vault_github_auth_backend.github.id
  team     = "developers"
  policies = ["default", "read-secrets"]

  depends_on = [vault_github_auth_backend.github]
}

# Example: Create a read-only policy for developers
resource "vault_policy" "read_secrets" {
  name = "read-secrets"

  policy = <<EOT
# Read-only access to shared secrets
path "${var.kv_mount_path}/data/shared/*" {
  capabilities = ["read", "list"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}
EOT
}

# Create audit logging for authentication
resource "vault_audit" "github_auth_log" {
  type = "file"
  path = "auth/github"

  options = {
    file_path = "/vault/logs/github-auth.log"
  }
}
