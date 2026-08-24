# Lentron

A small web UI for managing local tmux sessions remotely.

## Setup

Install Tailscale on the host and the devices that will access Lentron. Configure
`config.yaml` with your SSH account, SSH agent or key, and the Lentron user IDs
that may use Console:

```yaml
ssh:
  user: your_ssh_user
  agent_socket: /run/user/1000/keyring/ssh
  known_hosts_path: /home/your_user/.ssh/known_hosts

console_permissions:
  local:
    - 1
```

The SSH identity must be authorized on the local SSH server. Lentron keeps SSH
credentials on the server; they are never sent to the browser.

## Run

```sh
./start.sh
```

This starts Tailscale, shares Lentron privately over HTTPS, and starts the app.
It also shares a local Immich instance at port 2283 for the Immich embed. Use
`tailscale serve status` to find the URL to open on another device.
