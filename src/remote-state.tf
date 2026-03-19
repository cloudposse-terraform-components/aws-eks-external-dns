variable "eks" {
  type = object({
    eks_cluster_id                         = optional(string, null)
    eks_cluster_arn                        = optional(string, null)
    eks_cluster_endpoint                   = optional(string, null)
    eks_cluster_certificate_authority_data = optional(string, null)
    eks_cluster_identity_oidc_issuer       = optional(string, null)
    karpenter_iam_role_arn                 = optional(string, null)
  })
  description = "EKS cluster outputs. When set, bypasses remote-state lookup of eks/cluster."
  default     = null
  nullable    = true
}

module "eks" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "2.0.0"

  component = var.eks_component_name

  bypass = var.eks != null

  defaults = {
    eks_cluster_id                         = try(var.eks.eks_cluster_id, null)
    eks_cluster_arn                        = try(var.eks.eks_cluster_arn, null)
    eks_cluster_endpoint                   = try(var.eks.eks_cluster_endpoint, null)
    eks_cluster_certificate_authority_data = try(var.eks.eks_cluster_certificate_authority_data, null)
    eks_cluster_identity_oidc_issuer       = try(var.eks.eks_cluster_identity_oidc_issuer, null)
    karpenter_iam_role_arn                 = try(var.eks.karpenter_iam_role_arn, null)
  }

  context = module.this.context
}

module "dns_gbl_delegated" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "2.0.0"

  component   = "dns-delegated"
  environment = var.dns_gbl_delegated_environment_name

  context = module.this.context

  defaults = {
    zones = {}
  }
}

module "dns_gbl_primary" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "2.0.0"

  component   = "dns-primary"
  environment = var.dns_gbl_primary_environment_name

  context = module.this.context

  ignore_errors = true

  defaults = {
    zones = {}
  }
}

module "additional_dns_components" {
  for_each = { for obj in var.dns_components : obj.component => obj }
  source   = "cloudposse/stack-config/yaml//modules/remote-state"
  version  = "2.0.0"

  component   = each.value.component
  environment = coalesce(each.value.environment, "gbl")

  context = module.this.context
}
