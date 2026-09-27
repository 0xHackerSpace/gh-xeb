terraform {
  required_version = ">= 1.0"
  required_providers {
    vault = {
      source  = "hashicorp/vault"
      version = "~> 4.0"
    }
  }
}

provider "vault" {
  address = var.vault_addr
  token   = var.vault_token
}

# Enable KV v2 secrets engine
resource "vault_generic_secret" "kv_enable" {
  path = "sys/mounts/${var.kv_mount_path}"

  data_json = jsonencode({
    type    = "kv"
    version = 2
  })

  lifecycle {
    ignore_changes = [
      data_json,
    ]
  }
}

# Enable GitHub auth method
resource "vault_auth_backend" "github" {
  type = "github"
  path = "github"

  description = "GitHub authentication method"
}

# Configure GitHub auth backend
resource "vault_github_auth_backend" "github" {
  organization = var.github_organization
  base_url     = var.github_base_url

  depends_on = [vault_auth_backend.github]
}

# Map GitHub users to policies
resource "vault_github_user" "github_users" {
  for_each = var.github_users_mapping

  backend  = vault_github_auth_backend.github.id
  user     = each.key
  policies = each.value
}

# Map GitHub teams to policies
resource "vault_github_team" "github_teams" {
  backend  = vault_github_auth_backend.github.id
  team     = "contributors"
  policies = ["default"]
}

# Create policies from variables
resource "vault_policy" "policies" {
  for_each = var.vault_policies

  name   = each.key
  policy = each.value
}

# Create default policy
resource "vault_policy" "default" {
  name = "default"

  policy = <<EOT
# Default policy for authenticated users
path "${var.kv_mount_path}/data/personal/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "${var.kv_mount_path}/data/shared/*" {
  capabilities = ["read", "list"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}
EOT
}

# Create admin policy
resource "vault_policy" "admin" {
  name = "admin"

  policy = <<EOT
# Admin policy with full access
path "*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
EOT
}
