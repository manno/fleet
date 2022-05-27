terraform {
  required_providers {
    helm = {
      source = "hashicorp/helm"
      version = "2.5.1"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
      version = "2.11.0"
    }
  }
}

provider "kubernetes" {
  alias = "fleet"
  config_path = "~/.kube/config"
  config_context = var.fleet_context
}

provider "helm" {
  alias = "fleet"
  kubernetes {
    config_path = "~/.kube/config"
    config_context = var.fleet_context
  }
}

provider "kubernetes" {
  alias = "downstream"
  config_path = "~/.kube/config"
  config_context = var.fleet_downstream_context
}

provider "helm" {
  alias = "downstream"
  kubernetes {
    config_path = "~/.kube/config"
    config_context = var.fleet_downstream_context
  }
}

resource "helm_release" "fleet_fleet_crd" {
  provider = helm.fleet
  name       = "fleet-crd"
  namespace = "fleet-system"
  create_namespace = true
  chart = var.fleet_crd_chart
}

resource "helm_release" "fleet_fleet" {
  provider = helm.fleet
  depends_on = [helm_release.fleet_fleet_crd]
  name       = "fleet"
  namespace = "fleet-system"
  chart = var.fleet_chart
}

resource "helm_release" "fleet_token" {
  provider = helm.fleet
  depends_on = [helm_release.fleet_fleet]
  name       = "fleet-token-creator"
  chart      = "./fleet-token-creator"
  wait_for_jobs = true
}

data "kubernetes_secret" "downstream_values" {
  provider = kubernetes.fleet
  depends_on = [helm_release.fleet_token]
  metadata {
    namespace = "fleet-local"
    name = "fleet-token"
  }
}

data "kubernetes_service_account" "default" {
  provider = kubernetes.fleet
  metadata {
    name = "default"
    namespace = "kube-system"
  }
}

data "kubernetes_secret" "upstream_ca" {
  provider = kubernetes.fleet
  metadata {
    namespace = "kube-system"
    name = data.kubernetes_service_account.default.default_secret_name
  }
}

resource "helm_release" "fleet_agent" {
  provider = helm.downstream
  name       = "fleet-agent"
  namespace = "fleet-system"
  create_namespace = true
  chart = var.fleet_agent_chart

  set_sensitive {
    name  = "apiServerCA"
    value = var.fleet_ca_certificate
  }
  set {
    name  = "apiServerURL"
    value = var.fleet_api_url
  }
  set {
    name  = "clusterNamespace"
    value = "fleet-local"
  }
  set {
    name  = "systemRegistrationNamespace"
    value = "fleet-clusters-system"
  }
  set_sensitive {
    name  = "token"
    value = yamldecode(data.kubernetes_secret.downstream_values.data.values).token
  }
}
