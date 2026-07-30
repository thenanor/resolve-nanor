output "public_ip" {
  description = "Public IP of the instance"
  value       = aws_instance.this.public_ip
}

output "app_url" {
  description = "Where the app will be reachable after the first deploy"
  value       = "http://${aws_instance.this.public_ip}:${var.app_port}"
}

output "ssh_command" {
  description = "SSH into the instance"
  value       = "ssh -i ${trimsuffix(var.ssh_public_key_path, ".pub")} ec2-user@${aws_instance.this.public_ip}"
}
