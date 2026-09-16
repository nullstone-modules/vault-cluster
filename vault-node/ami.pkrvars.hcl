# Extra regions Packer copies the AMI into, besides the bake region.
# The bake region already has the image. If it appears here, Packer skips that copy.
ami_regions = [
  "us-east-1",
  "us-east-2",
  "us-west-1",
  "us-west-2",
]

# Launch permission for this AWS Organization.
ami_org_arns = [
  "arn:aws:organizations::031864064541:organization/o-4zofqm1ay9",
]

# "all" makes the AMI publicly launchable. Copied to ami_regions as well.
ami_groups = ["all"]
