# Governed entries: one keyed object per workload. Owner team is authoritative.
workloads = {
  orders-api = {
    owner        = "orders-team"
    instance_set = "standard-4"
    min_replicas = 3
    max_replicas = 30
    memory_mb    = 2048
    resources = {
      cpu       = 500
      memory_mb = 2048
    }
    labels = {
      team = "orders-team"
      tier = "prod"
    }
  }
  payments-gateway = {
    owner        = "payments-team"
    instance_set = "standard-8"
    min_replicas = 4
    max_replicas = 16
    memory_mb    = 4096
    resources = {
      cpu       = 1000
      memory_mb = 4096
    }
    labels = {
      team = "payments-team"
      tier = "prod"
    }
  }
  inventory-projector = {
    owner        = "inventory-team"
    instance_set = "standard-2"
    min_replicas = 2
    max_replicas = 6
    memory_mb    = 1024
    resources = {
      cpu       = 250
      memory_mb = 1024
    }
    labels = {
      team = "inventory-team"
      tier = "prod"
    }
  }
}
