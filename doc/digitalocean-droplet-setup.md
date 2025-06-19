# DigitalOcean Droplet Setup

DigitalOcean allows for easy server setup. I recommend using a *Droplet* to run Linux, which allows you to run arbitrary code, multiple Docker containers, etc. This doc will walk you through the process of setting up a Droplet with ByteGrader. 

> **Note:** you will need to purchase a domain name (e.g. from [Namecheap](https://www.namecheap.com/)) for your server, as HTTPS requires a unique domain name.

## Purchase and Set Up Droplet

* Log into [DigitalOcean](https://www.digitalocean.com/), select/create a project, and click **Create > Droplets**
* Choose Region: Pick closest to you/your students
* Choose Image: Latest **Ubuntu LTS** version
* Choose Size (start with the most inexpensive option):
    * Basic
    * 1 GB memory, 1 vCPU
    * This should handle ~10 concurrent grading jobs

For *authentication*, I recommend using an *SSH Key*, as it will mitigate brute-force attacks by someone trying to guess your root password. However, if you lose the SSH private key file from your local machine (e.g. wipe your OS without backup or lose your laptop), you will lose access to the server. If that happens, you will need to use the [DigitalOcean Recovery Console](https://docs.digitalocean.com/support/i-lost-the-ssh-key-for-my-droplet/) to regain access.

To generate a public/private SSH key pair, open a console (Bash, PowerShell, Zsh, etc.), make sure you have *OpenSSH* installed (e.g. [for Ubuntu](https://documentation.ubuntu.com/server/how-to/security/openssh-server/index.html)). Navigate into your `.ssh/` folder and run the `ssh-keygen` tool. Note that you should give your key pair a unique name (with `-f`) in case you want to generate more than one (i.e. one for each Droplet).

```sh
cd ~/.ssh/
ssh-keygen -t ed25519 -C "ByteGrader Droplet" -f id_ed25519_droplet
```

You'll then be to create a passphrase for the key, which I recommend you do. This will generate two files: `id_ed25519_droplet` and `id_ed25519_droplet.pub`. Print out the public key:

```sh
cat id_ed25519_droplet.pub
```

Copy the output (entire line). Back in your Droplet setup, click **SSH Key** and **Add SSH Key**. In the *SSH key content* field, paste your public key line. In *Name*, give your key a name that corresponds to your client (e.g. "My Laptop Login").

Feel free to purchase any add-ons if needed and give your server a unique hostname. Wait a few minutes while the Droplet setup completes.

Once it's done, you should be back in your DigitalOcean project dashboard. Note the IP address of your server!

Log into your server via SSH. Change `id_ed25519_droplet` to the name of your *SSH private key file, and change `<SERVER_IP_ADDRESS>` to the IP address of your server.

```sh
cd ~/.ssh/
ssh -i id_ed25519_droplet root@<SERVER_IP_ADDRESS>
```

## Configure DNS

We will assume that you want the Droplet to handle grading for multiple courses. So, we will create a subdomain for each course. For example, if `bytegrader.com` is the main domain for the grader, we will redirect `bytegrader.com` to the ByteGrader GitHub repo (using nginx), but `esp32-iot.bytegrader.com` will be the URL for the autograder.

Log in to your domain name provider and click to manage your domain for your grader. Add the following records (ask ChatGPT for specifics if you don't know how to do this for your particular domain name provider). Note that `<SUBDOMAIN>` is your course tag (e.g. `esp32-iot`) and `<YOUR_DROPLET_IP>` is the IP address we got for our Droplet in the previous step.

| Type  |  Host | Value | TTL |
|-------|-------|-------|-----|
| A     |  @    | <YOUR_DROPLET_IP> | Automatic |
| A     |  www  | <YOUR_DROPLET_IP> | Automatic |
| A     | <SUBDOMAIN> | <YOUR_DROPLET_IP> | Automatic |

## Set Up Server

To start, we'll create a new *bytegrader* user (so that we don't run everything as root), clone the repository, and configure the server. You should only need to do this once, and it should be done as *root*.

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

## Deploy ByteGrader Server App

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

## Enable SSL

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

## Update Process

Once the server is running, you can update ByteGrader (with minimal downtime) by logging into the server as the *bytegrader* user, stopping the container, updating the repository, and then calling the *deploy.sh* script again. Don't forget to give the script the location of your *app* directory!

```sh
ssh bytegrader@esp32-iot.bytegrader.com
cd ~/app
docker compose down
cd ~/bytegrader
git pull
./deploy/deploy.sh ~/app
```

Verify that the server is running with:

```sh
curl http://localhost:8080/health
```

## Test Grading

From your home computer (assuming you've whitelisted your public IP address), you can test submitting a dummy file for grading using the *test-stub.py* (which always returns a static grade/feedback so long as it receives a .zip file).

```sh
curl -X POST -H "X-API-Key: <API_KEY>" -F "file=@test/submission-make-c-hello.zip" https://<SUBDOMAIN>.<DOMAIN>/submit?assignment=test-stub
```

You should receive a "File submitted for grading" JSON message back from the server. Copy the *job_id* and check the status of the grading job:

```sh
curl -H "X-API-Key: <API_KEY>" https://<SUBDOMAIN>.<DOMAIN>/status/<JOB_ID>
```
