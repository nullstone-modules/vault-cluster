# MUST BE REJECTED - the wildcard sits directly under the tenant prefix, so it
# covers every tenant. This is the "temporary broad allow" that gets added when
# a correctly-written policy appears not to work.
path "kv/data/customers/*" {
  capabilities = ["read", "list"]
}
path "kv/metadata/customers/*" {
  capabilities = ["read", "list"]
}
