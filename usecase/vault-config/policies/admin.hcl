# Admin Policy
# Full access to all paths in Vault

path "*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
