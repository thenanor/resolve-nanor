variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "name" {
  description = "Name prefix for all resources"
  type        = string
  default     = "resolve"
}

variable "instance_type" {
  description = "EC2 instance type. t3.small (2 GB) fits app + Postgres comfortably; t3.micro works on free tier with swap."
  type        = string
  default     = "t3.small"
}

variable "app_port" {
  description = "Port the app is published on"
  type        = number
  default     = 3000
}

variable "ssh_public_key_path" {
  description = "Path to the SSH public key used to access the instance (generate with: ssh-keygen -t ed25519 -f ~/.ssh/resolve)"
  type        = string
  default     = "~/.ssh/resolve.pub"
}

variable "ssh_ingress_cidr" {
  description = "CIDR allowed to SSH. Default is open; tighten to <your-ip>/32 for anything long-lived."
  type        = string
  default     = "0.0.0.0/0"
}

variable "root_volume_gb" {
  description = "Root EBS volume size in GB"
  type        = number
  default     = 16
}
