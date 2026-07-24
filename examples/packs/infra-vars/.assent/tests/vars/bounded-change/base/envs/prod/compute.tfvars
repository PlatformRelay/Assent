# Governed entries: one keyed object per workload. Owner team is authoritative.
workloads = {
  orders-api = {
    owner        = "orders-team"
    instance_set = "standard-4"
    min_replicas = 3
    max_replicas = 12
    memory_mb    = 2048
  }
  payments-gateway = {
    owner        = "payments-team"
    instance_set = "standard-8"
    min_replicas = 4
    max_replicas = 16
    memory_mb    = 4096
  }
  inventory-projector = {
    owner        = "inventory-team"
    instance_set = "standard-2"
    min_replicas = 2
    max_replicas = 6
    memory_mb    = 1024
  }
}
