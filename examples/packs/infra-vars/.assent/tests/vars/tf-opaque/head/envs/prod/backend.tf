# HCL honesty fixture (EX-S05): a .tf resource/module BLOCK. Measured, not assumed
# (see expect.yaml): the .tf extension is not routed to the HCL parser at all
# today (only .tfvars is) — this file goes opaque via the YAML-producer default
# failing to parse HCL syntax, never a silent partial diff either way. Names are
# invented/generic (D-002): no real provider or company.
resource "example_compute_instance" "orders_api" {
  instance_type = "standard-8"
  replica_count = 4
}

module "networking" {
  source = "./modules/networking"
  cidr_block = "10.0.0.0/16"
}
