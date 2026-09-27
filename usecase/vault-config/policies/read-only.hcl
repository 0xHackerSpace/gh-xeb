# Read-Only Policy
# For third-party services or applications that need to read secrets only

# Read shared secrets
path "kv/data/shared/*" {
  capabilities = ["read", "list"]
}

# Read specific service credentials
path "kv/data/services/{{identity.entity.name}}/*" {
  capabilities = ["read"]
}

# Read database credentials (production)
path "database/creds/app-readonly" {
  capabilities = ["read"]
}

# Token lookup
path "auth/token/lookup-self" {
  capabilities = ["read"]
}

# Renew token
path "auth/token/renew-self" {
  capabilities = ["update"]
}
