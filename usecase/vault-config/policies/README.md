# Vault Policies

This directory contains Vault security policies (HCL format) that define what users and services can access in Vault.

## Policy Files

### Core Policies

- **admin.hcl** — Full access to all Vault resources
  - Use for: Vault administrators
  - Capabilities: create, read, update, delete, list on all paths

- **default.hcl** — Standard access for authenticated users
  - Use for: Regular developers and users
  - Capabilities:
    - Personal secrets: read/write/delete own secrets
    - Shared secrets: read-only
    - Token management: renew and lookup

### Role-Based Policies

- **developer.hcl** — Development team access
  - Read development secrets
  - Create personal dev configurations
  - Access database credentials (dev environment)
  - Issue development certificates

- **read-only.hcl** — Read-only access for applications
  - Use for: Third-party services, external integrations
  - Capabilities: read shared secrets, read-only database credentials

- **cicd.hcl** — CI/CD pipeline access
  - Use for: GitHub Actions, Jenkins, GitLab CI runners
  - Capabilities:
    - Read build configurations
    - Write artifacts and logs
    - Issue temporary deployment credentials
    - Generate service certificates

## How to Use Policies

### Load a Policy into Vault

```bash
# Using Vault CLI
vault policy write admin usecase/vault-config/policies/admin.hcl

vault policy write default usecase/vault-config/policies/default.hcl

vault policy write developer usecase/vault-config/policies/developer.hcl

vault policy write cicd usecase/vault-config/policies/cicd.hcl
```

### Assign Policies to GitHub Users

```bash
# Via Terraform (in terraform/main.tf)
vault_github_user "admin_user" {
  backend  = "github"
  user     = "admin_username"
  policies = ["admin"]
}

# Via CLI
vault write auth/github/map/users/admin_username value=admin
```

### Assign Policies to GitHub Teams

```bash
# Via Terraform (in terraform/github-auth.tf)
vault_github_team "dev_team" {
  backend  = "github"
  team     = "developers"
  policies = ["default", "developer"]
}

# Via CLI
vault write auth/github/map/teams/developers value=default,developer
```

## Policy Syntax Reference

### Path Matching

```hcl
# Exact match
path "kv/data/exact/path" { ... }

# Wildcard match (one level)
path "kv/data/*/secret" { ... }

# Recursive wildcard (all subpaths)
path "kv/data/secrets/*" { ... }

# Entity interpolation (dynamic paths)
path "kv/data/personal/{{identity.entity.name}}/*" { ... }
```

### Capabilities

| Capability | Description |
|------------|-------------|
| create | Create a secret |
| read | Read a secret |
| update | Modify a secret |
| delete | Delete a secret |
| list | List paths at a location |
| deny | Explicitly deny access |

## Examples

### Example: Deploy-Only Policy

```hcl
path "kv/data/shared/deploy/*" {
  capabilities = ["read"]
}

path "database/creds/app-deploy" {
  capabilities = ["read"]
}
```

### Example: Database Admin Policy

```hcl
path "database/*" {
  capabilities = ["read", "update", "create", "delete"]
}

path "database/config/*" {
  capabilities = ["read", "update"]
}
```

### Example: Team-Specific Policy

```hcl
# Platform team can manage infrastructure secrets
path "kv/data/infra/{{identity.entity.name}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Can request temporary cloud credentials
path "aws/creds/platform-*" {
  capabilities = ["read"]
}
```

## Best Practices

1. **Principle of Least Privilege**: Users should have minimum necessary access
2. **Separate by Role**: Create dedicated policies for different job functions
3. **Use Entity Interpolation**: Reference user identity in paths for isolation
4. **Audit Access**: Enable audit logging to track policy usage
5. **Regular Review**: Periodically review and update policies

## Loading Policies with Terraform

The Terraform configuration in the `terraform/` directory can automatically load these policies:

```hcl
resource "vault_policy" "developer" {
  name = "developer"
  policy = file("${path.module}/../policies/developer.hcl")
}
```

## Troubleshooting

### Permission Denied Errors

1. Check the user's assigned policies:
   ```bash
   vault token lookup
   ```

2. Check the policy definition:
   ```bash
   vault policy read policy-name
   ```

3. Validate policy syntax:
   ```bash
   vault policy write test-policy - < policies/test.hcl
   ```

### Policies Not Taking Effect

- GitHub team membership may be cached; wait a few minutes
- Renew your token to get new policies:
  ```bash
  vault token renew
  ```

## References

- [Vault Policies Documentation](https://www.vaultproject.io/docs/concepts/policies)
- [GitHub Auth Method](https://www.vaultproject.io/docs/auth/github)
- [Policy Examples](https://www.vaultproject.io/docs/concepts/policies#policy-examples)
