# Infrastructure — EC2 via Terraform

Provisions everything the app needs on AWS: an Amazon Linux 2023 instance
with Docker + compose preinstalled, a security group (SSH + app port),
and your SSH key.

## Prerequisites

- Terraform >= 1.5 (`brew tap hashicorp/tap && brew install hashicorp/tap/terraform`)
- AWS CLI configured (`aws configure`) — and your billing alarm set FIRST
- An SSH key for the instance:
  ```bash
  ssh-keygen -t ed25519 -f ~/.ssh/resolve -N ""
  ```

## Usage

```bash
cd terraform
terraform init
terraform plan          # review what will be created
terraform apply         # ~1 min; prints the IP, app URL, and ssh command
```

Give user-data ~2 minutes after apply, then verify:

```bash
$(terraform output -raw ssh_command) 'docker --version && docker compose version'
```

Wire CI deploys with the outputs:

if you have multiple users configured with gh, make sure to login with the correct user or token:
```bash
echo "ghp_yourToken" | gh auth login --hostname github.com --with-token
gh auth switch --hostname github.com --user your_user
```

and then run:
```bash
gh secret set EC2_HOST --body "$(terraform output -raw public_ip)"
gh secret set EC2_SSH_PRIVATE_KEY < ~/.ssh/resolve
```

## App secrets (`.env`)

`.github/workflows/deploy.yml` rsyncs the repo to the instance but
deliberately **excludes `.env`** — deploy syncs code, not secrets. There is
no GitHub Actions secret that reaches the container (`EC2_HOST` and
`EC2_SSH_PRIVATE_KEY` are only used to SSH in and run `docker compose up`);
`docker compose` on the host reads whatever `.env` already sits next to
`docker-compose.yml` there, so it has to be placed manually, once, per
instance:

```bash
scp -i ~/.ssh/resolve .env ec2-user@$(terraform output -raw public_ip):/home/ec2-user/resolve/.env
```

(Run this after the first deploy has created `/home/ec2-user/resolve/` — or `ssh`
in and `mkdir -p` it yourself first.)

This is required for `ANTHROPIC_API_KEY` (the triage service refuses to
start without it) and any other value you don't want defaulted — see
`.env.example` at the repo root for the full list. Re-run the `scp` after
rotating a key or replacing the instance; deploys never touch this file.

## Tear down

```bash
terraform destroy       # removes the instance, SG, and key pair
```

## Notes

- State stays local (`terraform.tfstate`) — fine for a single-person
  course project; remote state is a team concern.
- `terraform.tfvars`, state files, and `.terraform/` are gitignored;
  commit the `.tf` files and the lock file.
- Costs: t3.small ≈ $0.02/h + a few GB of gp3. `terraform destroy` when
  not using it, or stop the instance between classes.
