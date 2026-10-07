# MUST BE REJECTED - shared cluster: the wildcard sits under the env prefix, so it
# covers every env, or under one env, so it covers every tenant in it.
path "kv/data/envs/*" {
  capabilities = ["read", "list"]
}
path "kv/data/envs/+/customers/*" {
  capabilities = ["read"]
}
path "kv/metadata/envs/dev/customers/*" {
  capabilities = ["read", "list"]
}
