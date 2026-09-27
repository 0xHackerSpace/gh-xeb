variable "vault_addr" {
  description = "Vault server address"
  type        = string
  default     = "https://vault.example.com:8200"
}

variable "vault_token" {
  description = "Vault root token or admin token"
  type        = string
  sensitive   = true
}

variable "github_organization" {
  description = "GitHub organization for authentication"
  type        = string
}

variable "github_token" {
  description = "GitHub personal access token for managing teams/users"
  type        = string
  sensitive   = true
}

variable "github_base_url" {
  description = "GitHub base URL (for GitHub Enterprise)"
  type        = string
  default     = "https://github.com"
}

variable "vault_policies" {
  description = "Map of policy names to their HCL content"
  type        = map(string)
  default     = {}
}

variable "kv_mount_path" {
  description = "Path for KV v2 secrets engine"
  type        = string
  default     = "kv"
}

variable "github_users_mapping" {
  description = "Map GitHub users to Vault policies"
  type        = map(list(string))
  default = {
    "developer" = ["default"]
    "admin"     = ["admin"]
  }
}
