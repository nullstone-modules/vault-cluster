ui = false

disable_mlock = false

listener "tcp" {
  address         = "0.0.0.0:8200"
  cluster_address = "0.0.0.0:8201"
  tls_disable     = 1
}

log_level  = "info"
log_format = "json"
