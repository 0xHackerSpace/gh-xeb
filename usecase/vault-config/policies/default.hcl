# Default Policy
# Standard access for authenticated users

# Personal secrets - full access
path "kv/data/personal/{{identity.entity.name}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Shared secrets - read-only
path "kv/data/shared/*" {
  capabilities = ["read", "list"]
}

# Token self-management
path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/revoke-self" {
  capabilities = ["update"]
}

# List metadata of secrets
path "kv/metadata/*" {
  capabilities = ["list"]
}
