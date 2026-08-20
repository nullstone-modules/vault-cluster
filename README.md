# Secrets Vault Cluster

Reusable tenant-isolated secrets on HashiCorp Vault Community Edition.

Isolation is path and ACL based (`kv/customers/{tenant}/*`). This is not Vault
Enterprise namespaces.

```bash
./setup.sh
```

Operator guide: [vault-cluster-readme.md](vault-cluster-readme.md)

| Path | Role |
|---|---|
| `config/` `scripts/` `tests/` | Shared Vault behaviour |
| `local/` | Docker Compose (implemented) |
| `aws/` `gcp/` `azure/` | Future targets |
