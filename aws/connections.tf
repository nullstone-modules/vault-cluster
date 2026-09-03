data "ns_connection" "network" {
  name     = "network"
  contract = "network/aws/vpc"
}

data "ns_connection" "s3_bucket" {
  name     = "s3_bucket"
  contract = "datastore/aws/s3"
}

data "ns_connection" "unseal_key" {
  name     = "unseal_key"
  contract = "datastore/aws/kms"
}

locals {
  vpc_id             = data.ns_connection.network.outputs.vpc_id
  private_subnet_ids = data.ns_connection.network.outputs.private_subnet_ids

  snapshot_bucket_name = data.ns_connection.s3_bucket.outputs.db_hostname
  snapshot_bucket_arn  = data.ns_connection.s3_bucket.outputs.db_arn
  snapshot_bucket_url  = "${data.ns_connection.s3_bucket.outputs.db_protocol}://${data.ns_connection.s3_bucket.outputs.db_hostname}"
  snapshot_kms_key_arn = try(data.ns_connection.s3_bucket.outputs.kms_key_arn, "")

  unseal_kms_key_arn = data.ns_connection.unseal_key.outputs.kms_key_arn
  unseal_kms_key_id  = data.ns_connection.unseal_key.outputs.kms_key_id

  vpc_cidr = data.ns_connection.network.outputs.vpc_cidr

  vault_api_port     = 8200
  vault_cluster_port = 8201
  vault_health_port  = 8210
  snapshot_prefix    = "vault-snapshots"
}
