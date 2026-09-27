# Vault Configuration with Terraform

This directory contains Terraform configuration to set up a HashiCorp Vault server with GitHub authentication.

## Features

- **GitHub Authentication**: Users authenticate via GitHub tokens
- **Policy-based Access Control**: Teams and users map to Vault policies
- **KV v2 Secrets Engine**: Secure secrets storage
- **Audit Logging**: Track GitHub authentication events
- **Multi-team Support**: Admins, developers, and contributors with different access levels

## Files

- **main.tf** — Core Vault setup (KV engine, policies, GitHub auth backend)
- **github-auth.tf** — GitHub-specific configuration (teams, users, policies)
- **variables.tf** — Variable definitions
- **outputs.tf** — Output values
- **terraform.tfvars.example** — Example values (copy to terraform.tfvars)

## Prerequisites

1. **Terraform** >= 1.0
2. **Vault** server running and accessible
3. **GitHub organization** with admin access
4. **GitHub personal access token** with `admin:org_hook` scope

## Setup

### 1. Configure Variables

```bash
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your values
```

Or use environment variables:

```bash
export TF_VAR_vault_addr="https://your-vault.com:8200"
export TF_VAR_vault_token="s.your-root-token"
export TF_VAR_github_organization="your-org"
export TF_VAR_github_token="ghp_your-token"
```

### 2. Initialize Terraform

```bash
terraform init
```

### 3. Plan and Apply

```bash
terraform plan
terraform apply
```

## Usage

### Login to Vault via GitHub

```bash
export VAULT_ADDR="https://your-vault.com:8200"
vault login -method=github token=$GITHUB_TOKEN
```

### Access Secrets

```bash
# Read a personal secret
vault kv get kv/personal/my-secret

# List shared secrets
vault kv list kv/shared
```

## GitHub Team Mapping

### Default Configuration

| Team | Policies | Capabilities |
|------|----------|--------------|
| **admins** | admin | Full access |
| **developers** | default, read-secrets | Read shared secrets, manage personal |
| **contributors** | default | Basic access |

### Customize Teams

Edit `github-auth.tf` to add or modify team mappings:

```hcl
resource "vault_github_team" "your_team" {
  backend  = vault_github_auth_backend.github.id
  team     = "your-team-name"
  policies = ["policy1", "policy2"]
}
```

## Policies

### Default Policy

- Read/write personal secrets: `kv/data/personal/*`
- Read shared secrets: `kv/data/shared/*`
- Renew tokens

### Admin Policy

- Full access to all paths

### Custom Policies

Add custom policies via `vault_policies` variable:

```hcl
vault_policies = {
  "my-policy" = "path \"kv/data/custom/*\" { capabilities = [\"read\", \"list\"] }"
}
```

## Troubleshooting

### GitHub Auth Not Working

Check that the GitHub token has proper scopes:
- `admin:org_hook` — Required for organization access
- `read:org` — Read organization data

### Policy Errors

Validate policy syntax:

```bash
vault policy write test-policy - << EOF
path "kv/data/*" {
  capabilities = ["read"]
}
EOF
```

### Token Expired

Renew your token:

```bash
vault token renew
```

## References

- [Vault GitHub Auth Method](https://www.vaultproject.io/docs/auth/github)
- [Vault KV v2 Secrets Engine](https://www.vaultproject.io/docs/secrets/kv/kv-v2)
- [Terraform Vault Provider](https://registry.terraform.io/providers/hashicorp/vault/latest/docs)
