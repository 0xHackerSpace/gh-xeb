# Developer Policy
# Access for development team members

# Read and list development secrets
path "kv/data/dev/*" {
  capabilities = ["read", "list"]
}

# Create personal development configs
path "kv/data/dev/personal/{{identity.entity.name}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Read shared development resources
path "kv/data/shared/dev/*" {
  capabilities = ["read", "list"]
}

# Database credentials for dev
path "database/creds/dev-app" {
  capabilities = ["read"]
}

# PKI for development certificates
path "pki/issue/dev-*" {
  capabilities = ["create", "update"]
}

# Token self-management
path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}
