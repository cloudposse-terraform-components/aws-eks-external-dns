output "metadata" {
  value       = local.enabled ? module.external_dns[0].metadata : null
  description = "Block status of the deployed release"
}
