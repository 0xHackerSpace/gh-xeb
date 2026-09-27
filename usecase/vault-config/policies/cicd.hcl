# CI/CD Pipeline Policy
# Access for continuous integration and deployment pipelines

# Read build and deploy configurations
path "kv/data/cicd/*" {
  capabilities = ["read", "list"]
}

# Write build artifacts and logs
path "kv/data/cicd/artifacts/{{identity.entity.name}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

# Access database credentials for migrations
path "database/creds/cicd-runner" {
  capabilities = ["read"]
}

# Issue short-lived credentials for deployment
path "database/creds/deploy-*" {
  capabilities = ["read"]
}

# Generate PKI certificates for services
path "pki/issue/cicd-*" {
  capabilities = ["create", "update"]
}

# Read deployment secrets
path "kv/data/shared/deploy/*" {
  capabilities = ["read", "list"]
}

# Token management
path "auth/token/renew-self" {
  capabilities = ["update"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}
