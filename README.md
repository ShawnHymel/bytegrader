# ByteGrader Autograder Framework

<img src=".images/bytegrader-logo-text_2000px.png" alt="ByteGrader logo" width="1000" />

**ByteGrader** is an open-source, modular autograder designed to evaluate programming assignments in embedded systems, IoT, and edge AI. Built to support a variety of programming languages, it uses containerized environments to reliably compile, run, and assess student submissions.

This project was created to streamline the grading process for technical coursework, offering:
- Flexible assignment configurations
- Support for custom test scripts and code pattern checks
- Reproducible Docker-based environments for isolation and consistency

While ByteGrader is optimized for embedded systems and hardware-centric courses, its architecture is general enough to be extended to other domains and languages.

## Key Features

- **Language-agnostic grading engine** (Python, C, etc.)
- **Docker-based execution** to isolate and reproduce environments
- **Pluggable modules** for different courses or assignments
- **Code analysis tools** to verify API usage, function calls, or design patterns

## Example Use Cases

- Grading STM32 or ESP32 firmware assignments for embedded systems courses
- Evaluating IoT device data parsing and MQTT communication tasks
- Verifying Python scripts for edge AI inference pipelines
- Ensuring students follow proper function use and coding standards

## Clients

The following clients have been tested with ByteGrader. Please see their respective documentation for version compatibility.

* [ByteGrader Client for LearnDash](https://github.com/ShawnHymel/bytegrader-client-learndash)

## Getting Started

If you want to test a grader locally, see the [Developing a Grader](#developing-a-grader) section to see how to spin up a container with a grader. Setting up the full server will require a little more effort.

### Domain and Hosting Setup

For the full server, you will need to buy a domain or configure a domain with a subdomain (e.g. *esp32-iot.bytegrader.com*) for SSL/TLS certificate signing. I recommend [Namecheap](https://www.namecheap.com/).

With a domain, you can then set up the hosting service. This can be a self-maintained server from your home/office or a paid virtual private server (VPS). See the following guides for buying/configuring a server:

 * [DigitalOcean Droplet Setup](doc/digitalocean-droplet-setup.md)

Make sure you can get root shell access to your server (e.g. through SSH).

### Configure DNS

We will assume that you want the server to handle grading for multiple courses. So, we will create a subdomain for each course. For example, if `bytegrader.com` is the main domain for the grader, we will redirect `bytegrader.com` to the ByteGrader GitHub repo (using nginx), but `esp32-iot.bytegrader.com` will be the URL for the autograder.

Log in to your domain name provider and click to manage your domain for your grader. Add the following records (ask ChatGPT for specifics if you don't know how to do this for your particular domain name provider). Note that `<SUBDOMAIN>` is your course tag (e.g. `esp32-iot`) and `<YOUR_SERVER_IP>` is the IP address we got for our server in the previous step.

| Type  |  Host | Value | TTL |
|-------|-------|-------|-----|
| A     |  @    | <YOUR_SERVER_IP> | Automatic |
| A     |  www  | <YOUR_SERVER_IP> | Automatic |
| A     | <SUBDOMAIN> | <YOUR_SERVER_IP> | Automatic |

### Clone Your Assignments Repo (Optional)

If you are using a private assignments repo rather than the default graders bundled with ByteGrader, clone it now. You will need a [GitHub Personal Access Token (PAT)](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens) with read access to the repo:
```sh
git clone https://<PAT>@github.com/<your-org>/<your-assignments-repo> /home/bytegrader/my-assignments
```

The PAT is only needed for this one-time clone. Once the repo is on the server, it is not needed again unless you redeploy.

### Configure

Edit the [config.yaml](./config.yaml) file:

```sh
nano config.yaml
```
 
At minimum you must set:

 * **domain** - Your root domain (e.g. `bytegrader.com`)
 * **subdomain** - Your course subdomain (e.g. `esp32-iot`)
 * **ssl_email** - Your email for SSL certificate notifications
 * **graders_local_path** - Path to your graders directory (see below)
 * **grader_images** - Map of assignment names to Docker image tags

For `graders_local_path`, use one of the following depending on your setup:

```yaml
# Use the default graders bundled with ByteGrader:
graders_local_path: /home/bytegrader/bytegrader/graders

# Use a private assignments repo you cloned above:
graders_local_path: /home/bytegrader/my-assignments/graders
```

Optionally configure security settings:

```yaml
# Restrict access to specific IP addresses:
ip_whitelist: ["203.0.113.5", "192.168.1.0/24"]

# Require an API key for all requests:
api_keys: ["your-secret-key-here"]
```

### Install

Run the install script as *root*:

```sh
bash install.sh
```

This will:
 1. Install system dependencies and Docker
 2. Create the `bytegrader` system user
 3. Build grader Docker images
 4. Build and start the ByteGrader API container
 5. Configure nginx
 6. Obtain and configure SSL certificates (requires DNS to be propagated)

Verify the server is running:

```sh
curl https://<SUBDOMAIN>.<DOMAIN>/health
```

> *Note*: this script will create a user in Linux with the name set by `bytegrader_user` in *config.yaml*. The user is created without a password, so the only way to log in as that user is to switch to it from root (e.g. `su - bytegrader`).

### Test With Remote Client

With the server running, you should be able to send test submissions to the `/submit` endpoint from one of your clients on the approved IP address whitelist.

```sh
curl -X POST \
  -H "X-API-Key: <API_KEY>" \
  -H "X-Username: test-user" \
  -F "file=@test/make-c-add/submission.zip" \
  "https://<SUBDOMAIN>.<DOMAIN>/submit?assignment=make-c-add"
```

You should receive a "File submitted for grading" JSON message back from the server. Copy the *job_id* and check the status of the grading job:

```sh
curl -H "X-API-Key: <API_KEY>" \
  -H "X-Username: test-user" \
  "https://<SUBDOMAIN>.<DOMAIN>/status/<JOB_ID>"
```

## Updating

If you have a live server and want to do an update, you should consider doing a [blue-green deployment](https://en.wikipedia.org/wiki/Blue%E2%80%93green_deployment), which should help minimize downtime. With this strategy, instantiate a new server while your existing server is still running. Install ByteGrader and your course-specific grader(s) on the new server, test that the new server still works, switch the DNS entry to point to the new server, then bring down the old server. 

### Blue-Green Deployment

On the new server, clone this repo and your grader repo. Set the options in [config.yaml](/config.yaml). Then, run the install script (as *root*) but skip *nginx* (which requires DNS):

```sh
bash install.sh --skip-nginx
```

Test the new server (you can also try sending a known-good assignment for it to grade):

```sh
curl http://<NEW_IP>:8080/health
```

Switch your DNS *A Record* to the new server IP address. Wait for propagation, then run (as *root*):

```sh
bash install.sh --skip-build
```

Your new server should be now accepting submissions (via HTTPS), and you can bring down the old server.

## API Endpoints

See [the API endpoints page](/doc/api-endpoints.md) for a full list of endpoints and supported HTTP methods.

## Developing a Grader

See [Creating a Grader](doc/creating-a-grader.md) for more information.

## Update Process

Once the server is running, you can update ByteGrader (with minimal downtime) by logging into the server as the *bytegrader* user, stopping the container, updating the repository, and then calling the *deploy.sh* script again. Don't forget to give the script the location of your *app* directory!

```sh
ssh bytegrader@<SUBDOMAIN>.<DOMAIN>
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

From your home/office computer (assuming you've whitelisted your public IP address), you can test submitting a dummy file for grading using the *test-stub* grader (which always returns a static grade/feedback so long as it receives a valid .zip file).

```sh
curl -X POST -H "X-API-Key: <API_KEY>" -H "X-Username: test-user" -F "file=@test/make-c-add/submission.zip" https://<SUBDOMAIN>.<DOMAIN>/submit?assignment=make-c-add
```

You should receive a "File submitted for grading" JSON message back from the server. Copy the *job_id* and check the status of the grading job:

```sh
curl -H "X-API-Key: <API_KEY>" -H "X-Username: test-user" https://<SUBDOMAIN>.<DOMAIN>/status/<JOB_ID>
```

You can watch the real-time logs of the server with:

```sh
cd /home/bytegrader/app
docker compose logs -f
```

## Security Best Practices

- Never commit API keys, passwords, or certificates
- Use environment variables for sensitive configuration
- Keep dependencies updated
- Enable branch protection on main

## Notes

### Check Logs

Docker Compose keeps a running set of logs. You can view them by logging into the server, navigating to the *app/* directory, and running:

```sh
cd /home/bytegrader/app/
docker compose logs
```

You can also watch logs in realtime with:

```sh
docker compose logs -f
```

### Check the Queue

You can view the queue from an approved IP address and with the correct API key:

curl -H "X-API-Key: <API_KEY>" https://<SUBDOMAIN>.<DOMAIN>/queue

### Server Health Check

There is a quick and dirty health check script you can run to get stats on the server (CPU, RAM, disk usage, etc.):

```sh
bash /home/bytegrader/app/health-check.sh
```

### Update Go Dependences

To update Go dependencies (i.e. if you import a new package in *main.go* or want to update package listings in *go.mod* and *go.sum*), run the following:

```sh
docker run --rm -v "$PWD/server":/app -w /app golang:1.24 go mod tidy
```

### Check Go Server Syntax

If you want to do a quick build of the Go server and throw away the build artifacts to check for basic syntax and build-time errros, just run a quick Go container:

```sh
docker run --rm -v "$PWD/server":/app -w /app golang:1.24 go build -o /dev/null .
```

### Update IP Whitelist

If the server is already running and you'd like to update the white list (e.g. so you can add another client for testing), edit the environment variables:

```sh
nano /home/bytegrader/.bytegrader_env
```

Once you've added/removed the desired IP addresses, redploy (which will read in the saved environment variables from that file):

```sh
cd /home/bytegrader/bytegrader
bash deploy/deploy.sh ../app
```

Note that if you need to get the "local" IP address (what the App container sees when making calls from the host server), as `127.0.0.1` and `localhost` won't often work, you can run `docker compose logs | grep "Security check"`. This will likely be `172.18.0.0/16` so the App container can see the host.

### Release Notes

Release notes are kept in [CHANGELOG.md](./CHANGELOG.md).

## Todo

 * Add full integration test with make/C grader example
 * Make multi-stage Docker build for adding in environments (e.g. Arduino, ESP-IDF)

## License

All code, unless otherwise specified, is subject to the [3-Clause BSD License](https://opensource.org/license/bsd-3-clause). See the [LICENSE](./LICENSE) file for more details.