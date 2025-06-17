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

## Configure ByteGrader

Make sure that you are SSH'd into your server. In the server, clone the ByteGrader repository and run the setup scripts. Note that `<DOMAIN>` is the domain you bought earlier (e.g. *bytegrader.com*), `<SUBDOMAIN>` is the subdomain you set in your DNS (e.g. `esp32-iot`), and `<EMAIL>` is your desired email address (for SSL certification notifications via [certbot](https://certbot.eff.org/)).

```
git clone https://github.com/ShawnHymel/bytegrader.git
cd bytegrader
chmod +x ./deploy/*.sh
./deploy/server-setup.sh <DOMAIN> <SUBDOMAIN> <EMAIL>
```

During the setup process, if OpenSSH asks you about modifying *sshd_config*, select the default "Keep the local version currently installed."

> **Note:** If the setup process fails, you might have to delete the *bytegrader* user and try again: `userdel -r bytegrader 2>/dev/null || true`

