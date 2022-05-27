variable "fleet_api_url" {
  type = string
}
variable "fleet_ca_certificate" {
  type = string
}
variable "fleet_context" {
  type = string
  default = "k3d-k3s-default"
}
variable "fleet_downstream_context" {
  type = string
  default = "k3d-k3s-second"
}
variable "fleet_crd_chart" {
  type = string
  default = "https://github.com/rancher/fleet/releases/download/v0.3.9/fleet-crd-0.3.9.tgz"
}
variable "fleet_chart" {
  type = string
  default = "https://github.com/rancher/fleet/releases/download/v0.3.9/fleet-0.3.9.tgz"
}
variable "fleet_agent_chart" {
  type = string
  default = "https://github.com/rancher/fleet/releases/download/v0.3.9/fleet-agent-0.3.9.tgz"
}
