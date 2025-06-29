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
