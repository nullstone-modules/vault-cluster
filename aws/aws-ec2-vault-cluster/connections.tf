data "ns_connection" "network" {
  name     = "network"
  contract = "network/aws/vpc"
}

data "ns_connection" "snapshots_bucket" {
  name     = "snapshots_bucket"
  contract = "datastore/aws/s3"
}

data "ns_connection" "unseal_key" {
  name     = "unseal_key"
  contract = "datastore/aws/kms"
}

locals {
  vpc_id   = data.ns_connection.network.outputs.vpc_id
  vpc_cidr = data.ns_connection.network.outputs.vpc_cidr

  snapshot_bucket_arn  = data.ns_connection.snapshots_bucket.outputs.db_arn
  snapshot_kms_key_arn = try(data.ns_connection.snapshots_bucket.outputs.kms_key_arn, "")

  unseal_kms_key_arn = data.ns_connection.unseal_key.outputs.kms_key_arn

  vault_api_port     = 8200
  vault_cluster_port = 8201
  vault_health_port  = 8210
  snapshot_prefix    = "vault-snapshots"
}
