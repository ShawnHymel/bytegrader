# Manual Installation

For most installations, you should use the *install.sh* script in the root directory. If, for some reason, you need to perform those steps manually, this document walks you through the process of installing ByteGrader on a server.

## Install Docker

To start, you'll need to install Docker (from [these instructions](https://docs.docker.com/engine/install/ubuntu/)). As root, install dependencies and add Docker's official GPG key:

```sh
apt-get update
apt-get install ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
```

Add the Docker repository to the Apt sources:

```sh
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" | \
  tee /etc/apt/sources.list.d/docker.list > /dev/null
apt-get update
```

Install Docker:

```sh
sudo apt-get install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

## Server Configuration

we'll create a new *bytegrader* user (so that we don't run everything as root), clone the repository, and configure the server. You should only need to do this once, and it should be done as *root*.

Make sure that you are SSH'd into your server (as *root*). Create the *bytegrader* user (this name is important, as the setup scripts assume you have such a user and home directory).

```sh
adduser --disabled-password --gecos "" bytegrader
usermod -aG docker bytegrader
```

You can optionally copy the SSH keys so you can remotely log into the server as either root or bytegrader.

```sh
mkdir -p /home/bytegrader/.ssh
cp /root/.ssh/authorized_keys /home/bytegrader/.ssh/
chown -R bytegrader:bytegrader /home/bytegrader/.ssh
chmod 700 /home/bytegrader/.ssh
chmod 600 /home/bytegrader/.ssh/authorized_keys
```

If you don't want to log in directly as *bytegrader*, you can log in as root and switch to *bytegrader*. 

```sh
su - bytegrader
```

### Server Setup

Make sure you are in the **bytegrader** user, and then clone the repo:

```sh
cd /home/bytegrader/
git clone https://github.com/ShawnHymel/bytegrader.git
cd bytegrader/
```

Feel free to check out a particular tag, version, or branch. (e.g. `git checkout v1.2`).

Make sure the setup scripts are executable:

```sh
chmod +x deploy/*.sh
```

Log back in as **root** (or enter `logout` to escape out of the `su - bytegrader` shell). Then, run the server setup script as a superuser:

```sh
cd /home/bytegrader/bytegrader/
bash deploy/setup-server.sh
```

This will walk you through the process of assigning several important environment variables that are used throughout the setup process:

 * **Main domain** - The main domain name you purchased earlier (e.g. bytegrader.com). Note that for now, this will redirect to `github.com/ShawnHymel/bytegrader`, as we only need the subdomain for our autograder endpoints.
 * **Course subdomain** - The server will set up a subdomain for your course's autograder endpoints. For example, `esp32-iot` will mean the full URL of the autograder is `https://esp32-iot.bytegrader.com`.
 * **Email** - Your email address for SSL certificate notifications (from [certbot](https://certbot.eff.org/))
 * **IP whitelist** - List of IP addresses (comma separated) that are allowed to connect to the server. Leave empty to allow all connections. Ideally, this should be the IPv4 and IPv6 addresses of your course site (LMS) and your personal, public IP address (so you can test from home/office).
 * **API key** - Secret key (password) used to authenticate clients connecting to the server. Ideally, only your LMS site should have the same key.

If *openssh-server* pops up asking you what to do with the existing *sshd_config* file, accept the default (keep the local version).

### Deploy ByteGrader Server App

Log in as the **bytegrader** user (e.g. `su - bytegrader`), make an *app/* directory, and run the deploy app. The *deploy.sh* script will copy the relevant files from the repo to the *app/* directory.

```sh
cd /home/bytegrader/
mkdir -p app/
cd bytegrader/
bash deploy/deploy.sh /home/bytegrader/app/
```

Once that runs, you can check to make sure that the grader container is reachable locally:

```sh
curl http://localhost:8080/health
```

This should show `{"status":"ok"}`.

Then, you can check to make sure that you can lookup your subdomain's IP address with:

```sh
nslookup <SUBDOMAIN>.<DOMAIN>
```

You can't make any requests yet, as you need to enable SSL.

### Enable SSL

Even though our app is running, we need to generate SSL certificates and get them signed (through Let's Encrypt). It also sets up *certbot* to renew certificates automatically. We needed to wait until now to run *setup-ssl.sh*, as we only just set up our domain and subdomain with the *deploy.sh* script.

Switch to the **root** user (`logout` or re-login via SSH) and run the *setup-ssl.sh* script:

```sh
cd /home/bytegrader/bytegrader
bash deploy/setup-ssl.sh
```

Hopefully, this completes successfully. You can check with:

```sh
curl https://<SUBDOMAIN>.<DOMAIN>/health
```

This should show `{"status":"ok"}`.

### Test With Remote Client

With the server running, you should be able to send test submissions to the `/submit` endpoint from one of your clients on the approved IP address whitelist. See [Test Grading](#test-grading) for more information.