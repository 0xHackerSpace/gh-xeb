output "vault_addr" {
  description = "Vault server address"
  value       = var.vault_addr
}

output "github_auth_path" {
  description = "Path where GitHub auth backend is mounted"
  value       = vault_auth_backend.github.path
}

output "kv_mount_path" {
  description = "Path where KV v2 secrets engine is mounted"
  value       = var.kv_mount_path
}

output "vault_policies" {
  description = "Vault policies created"
  value       = [for policy in vault_policy.policies : policy.name]
}

output "github_organization" {
  description = "GitHub organization configured for authentication"
  value       = var.github_organization
}

output "login_command" {
  description = "Command to login to Vault via GitHub"
  value       = "vault login -method=github token=<github_token>"
}

output "github_teams_configured" {
  description = "GitHub teams mapped to Vault policies"
  value = {
    "admins"     = vault_github_team.admins.policies
    "developers" = vault_github_team.developers.policies
    "contributors" = vault_github_team.github_teams.policies
  }
}
