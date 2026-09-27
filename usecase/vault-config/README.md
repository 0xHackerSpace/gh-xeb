# Vault Configuration

Complete setup for HashiCorp Vault with GitHub authentication, including Terraform infrastructure-as-code and security policies.

## Directory Structure

```
vault-config/
├── terraform/              # Terraform configuration for Vault setup
│   ├── main.tf            # Core Vault resources
│   ├── github-auth.tf     # GitHub authentication configuration
│   ├── variables.tf       # Input variables
│   ├── outputs.tf         # Output values
│   ├── terraform.tfvars.example  # Example variable values
│   └── README.md          # Terraform setup guide
│
├── policies/              # Vault security policies (HCL)
│   ├── admin.hcl         # Admin full access
│   ├── default.hcl       # Default user access
│   ├── developer.hcl     # Developer team access
│   ├── read-only.hcl     # Read-only access for applications
│   ├── cicd.hcl          # CI/CD pipeline access
│   └── README.md         # Policy documentation
│
└── README.md             # This file
```

## Quick Start

### 1. Set Up Vault with Terraform

```bash
cd terraform

# Copy example configuration
cp terraform.tfvars.example terraform.tfvars

# Edit with your values
nano terraform.tfvars

# Initialize and apply
terraform init
terraform plan
terraform apply
```

### 2. Configure GitHub Authentication

The Terraform configuration automatically sets up GitHub auth. You just need to:

1. Create a GitHub organization (or use existing)
2. Generate a personal access token with `admin:org_hook` scope
3. Add it to `terraform.tfvars`

### 3. Load Policies

Policies are automatically created by Terraform, or manually:

```bash
# Load default policy
vault policy write default policies/default.hcl

# Load developer policy
vault policy write developer policies/developer.hcl

# Load CI/CD policy
vault policy write cicd policies/cicd.hcl
```

### 4. Log In

```bash
export VAULT_ADDR="https://your-vault.com:8200"
vault login -method=github token=$GITHUB_TOKEN
```

## Features

### ✅ GitHub Authentication
- Users authenticate using GitHub personal access tokens
- Teams mapped to Vault policies
- Automatic policy updates when GitHub team membership changes

### ✅ Security Policies
- **Admin**: Full access (Vault operators)
- **Default**: User access (personal + shared secrets)
- **Developer**: Dev team access (dev secrets + database)
- **Read-Only**: Application access (shared secrets only)
- **CI/CD**: Pipeline access (build secrets + temporary creds)

### ✅ Secrets Management
- KV v2 secrets engine for key-value storage
- Personal secret isolation
- Shared secret repository

### ✅ Audit & Logging
- GitHub authentication events logged
- Full audit trail of secret access

## Authentication Methods

### Authenticate with GitHub

```bash
# Get your GitHub token
export GITHUB_TOKEN="ghp_your_token_here"

# Log in to Vault
vault login -method=github token=$GITHUB_TOKEN

# Verify authentication
vault token lookup
```

### Authenticate Programmatically

```bash
# Using gh-xeb CLI
gh xeb vault token

# Using curl
curl -X POST \
  -d '{"token":"$GITHUB_TOKEN"}' \
  https://your-vault.com/v1/auth/github/login
```

## Accessing Secrets

### Via CLI

```bash
# Read a personal secret
vault kv get kv/personal/my-secret

# Read a shared secret
vault kv get kv/shared/database-url

# List secrets
vault kv list kv/personal
```

### Via gh-xeb

```bash
# Using the extension
gh xeb vault get kv/data/prod/db

# With masking (default)
gh xeb vault get kv/data/prod/db --mask

# Reveal values
gh xeb vault get kv/data/prod/db --reveal
```

### Via Terraform

```hcl
data "vault_generic_secret" "database" {
  path = "kv/data/prod/db"
}

output "db_password" {
  sensitive = true
  value     = data.vault_generic_secret.database.data["password"]
}
```

## GitHub Team to Policy Mapping

Default mappings in `terraform/main.tf`:

| GitHub Team | Vault Policies | Use Case |
|-------------|---|---|
| admins | admin | Vault operators |
| developers | default, developer | Development team |
| contributors | default | Contributors/maintainers |

Customize in `terraform/github-auth.tf` by adding/modifying `vault_github_team` resources.

## Common Tasks

### Add a New User

```bash
# Via Terraform
# Add to terraform/main.tf
vault_github_user "new_user" {
  backend  = "github"
  user     = "github_username"
  policies = ["default"]
}

# Apply
terraform apply

# Or via CLI
vault write auth/github/map/users/github_username value=default
```

### Add a New Team

```bash
# Via Terraform - Edit terraform/github-auth.tf
resource "vault_github_team" "new_team" {
  backend  = "github"
  team     = "team-name"
  policies = ["default", "developer"]
}

# Apply
terraform apply
```

### Create a Custom Secret

```bash
vault kv put kv/shared/my-secret \
  username="user" \
  password="pass" \
  url="https://example.com"
```

### Revoke Access

```bash
# Remove user from GitHub team (automatic)
# Or manually revoke in Vault:
vault write auth/github/map/users/username value=""
```

## Environment Variables

For gh-xeb to work properly:

```bash
# Required
export VAULT_ADDR="https://your-vault.com:8200"
export VAULT_TOKEN="your-token"  # Or use GitHub auth

# Optional
export VAULT_NAMESPACE="namespace"  # If using Vault namespaces
export GITHUB_TOKEN="your-github-token"
```

## Troubleshooting

### GitHub Auth Not Working

1. Verify token has `admin:org_hook` scope:
   ```bash
   curl -H "Authorization: token $GITHUB_TOKEN" \
     https://api.github.com/user -v | grep X-OAuth-Scopes
   ```

2. Check GitHub org name matches configuration:
   ```bash
   terraform output github_organization
   ```

### Can't Access a Secret

1. Check your policies:
   ```bash
   vault token lookup
   ```

2. Check the policy allows the path:
   ```bash
   vault policy read your_policy
   ```

3. Test the path:
   ```bash
   vault kv list kv/personal
   ```

### Token Expired

```bash
vault token renew
```

### Vault Audit Logs

```bash
# View audit logs (requires file audit backend)
tail -f /vault/logs/github-auth.log
```

## Integration with gh-xeb

This Vault configuration works seamlessly with the gh-xeb CLI extension:

```bash
# Check Vault readiness
gh xeb doctor

# Get Vault token info
gh xeb vault token

# List mounts
gh xeb vault mounts

# Check capabilities
gh xeb vault can kv/data/shared/database

# Read a secret
gh xeb vault get kv/data/shared/database
```

## Security Best Practices

1. **Use Short-Lived Tokens**: Renew frequently
2. **Principle of Least Privilege**: Grant minimum necessary access
3. **Team-Based Access**: Use GitHub teams for policy assignment
4. **Audit Everything**: Enable audit logging
5. **Rotate Secrets**: Regularly rotate passwords and API keys
6. **Never Log Tokens**: Configure audit to redact sensitive data
7. **Secure Backups**: Encrypt backup data

## References

- [Vault Documentation](https://www.vaultproject.io/docs)
- [GitHub Auth Method](https://www.vaultproject.io/docs/auth/github)
- [KV v2 Secrets Engine](https://www.vaultproject.io/docs/secrets/kv/kv-v2)
- [Terraform Vault Provider](https://registry.terraform.io/providers/hashicorp/vault/latest)
- [gh-xeb Repository](https://github.com/0xHackerSpace/gh-xeb)

## License

Same as gh-xeb
