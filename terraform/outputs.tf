output "public_ip" {
  description = "Elastic IP of the instance"
  value       = aws_eip.this.public_ip
}

output "app_url" {
  description = "Where the API will be reachable after the first deploy"
  value       = "http://${aws_eip.this.public_ip}:${var.app_port}"
}

output "frontend_url" {
  description = "Where the UI will be reachable after the first deploy"
  value       = "http://${aws_eip.this.public_ip}:${var.frontend_port}"
}

output "ssh_command" {
  description = "SSH into the instance"
  value       = "ssh -i ${trimsuffix(var.ssh_public_key_path, ".pub")} ec2-user@${aws_eip.this.public_ip}"
}
