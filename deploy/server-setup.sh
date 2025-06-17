#!/bin/bash
# deploy/server-setup.sh - Set up ByteGrader server infrastructure
# Usage: ./deploy/server-setup.sh <main_domain> <course_subdomain> <email>
# Example: ./deploy/server-setup.sh bytegrader.com iot-esp32 you@example.com

set -e

# Check if we're running as root
if [ "$EUID" -ne 0 ]; then
    echo "❌ This script must be run as root"
    echo "💡 Try: sudo ./scripts/server-setup.sh <domain> <subdomain> <email>"
    exit 1
fi

# Check if correct number of arguments provided
if [ $# -ne 3 ]; then
    echo "Usage: $0 <main_domain> <course_subdomain> <email>"
    echo ""
    echo "Examples:"
    echo "  $0 bytegrader.com iot-esp32 you@example.com"
    echo "  $0 mydomain.com python101 admin@mydomain.com"
    echo ""
    echo "This will set up:"
    echo "  - Main domain (redirects to GitHub): https://MAIN_DOMAIN"
    echo "  - Course API: https://COURSE_SUBDOMAIN.MAIN_DOMAIN"
    exit 1
fi

# Parse command-line arguments
MAIN_DOMAIN="$1"
COURSE_SUBDOMAIN="$2"
EMAIL="$3"
COURSE_DOMAIN="${COURSE_SUBDOMAIN}.${MAIN_DOMAIN}"

echo "🚀 Setting up ByteGrader server infrastructure:"
echo "  Main domain: $MAIN_DOMAIN (redirects to GitHub repo)"
echo "  Course API: $COURSE_DOMAIN (autograder)"
echo "  SSL email: $EMAIL"
echo ""

# Confirm before proceeding
read -p "Continue with this configuration? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Setup cancelled."
    exit 1
fi

echo "📦 Updating system packages..."
apt update && apt upgrade -y

echo "🔧 Installing essential tools..."
apt install -y curl wget git vim htop ufw nginx certbot python3-certbot-nginx

echo "🐳 Installing Docker..."
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh && rm get-docker.sh
apt install -y docker-compose-plugin

echo "👤 Setting up bytegrader user..."
# Check if user already exists
if id "bytegrader" &>/dev/null; then
    echo "✅ bytegrader user already exists"
else
    echo "🆕 Creating bytegrader user..."
    adduser --disabled-password --gecos "" bytegrader
fi

# Always ensure user is in correct groups (safe to run multiple times)
usermod -aG docker bytegrader
usermod -aG sudo bytegrader

# Set up SSH keys for bytegrader user
if [ -d "/root/.ssh" ]; then
    echo "🔑 Setting up SSH keys for bytegrader user..."
    mkdir -p /home/bytegrader/.ssh
    cp /root/.ssh/authorized_keys /home/bytegrader/.ssh/ 2>/dev/null || true
    chown -R bytegrader:bytegrader /home/bytegrader/.ssh
    chmod 700 /home/bytegrader/.ssh
    chmod 600 /home/bytegrader/.ssh/authorized_keys 2>/dev/null || true
    echo "✅ SSH keys copied. You can now SSH directly as: ssh bytegrader@$COURSE_DOMAIN"
else
    echo "⚠️  No SSH keys found. You'll need to su - bytegrader from root"
fi

echo "📁 Setting up application directory..."
mkdir -p /home/bytegrader/app/{graders,uploads,logs}
chown -R bytegrader:bytegrader /home/bytegrader

echo "🌐 Creating nginx configuration from template..."
# Check if template exists
if [ ! -f "deploy/bytegrader.conf.template" ]; then
    echo "❌ deploy/bytegrader.conf.template not found!"
    echo "💡 Make sure you're running this from the bytegrader repository root"
    exit 1
fi

# Substitute variables in nginx template
sed -e "s/\${MAIN_DOMAIN}/$MAIN_DOMAIN/g" \
    -e "s/\${COURSE_DOMAIN}/$COURSE_DOMAIN/g" \
    -e "s/\${COURSE_SUBDOMAIN}/$COURSE_SUBDOMAIN/g" \
    deploy/bytegrader.conf.template > /etc/nginx/sites-available/bytegrader

# Enable the site
ln -sf /etc/nginx/sites-available/bytegrader /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

# Test nginx configuration
nginx -t

echo "🛡️ Configuring firewall..."
ufw allow ssh
ufw allow http
ufw allow https
ufw --force enable

echo "🌐 Starting nginx..."
systemctl enable nginx
systemctl restart nginx

echo "📋 Copying deployment files to bytegrader user..."
# Copy deployment files to bytegrader's home directory
sudo -u bytegrader cp -r deploy/* /home/bytegrader/app/
chown -R bytegrader:bytegrader /home/bytegrader/app

# Substitute variables in docker-compose.yml
sudo -u bytegrader sed -i "s/\${COURSE_SUBDOMAIN}/$COURSE_SUBDOMAIN/g" /home/bytegrader/app/docker-compose.yml

echo "🔧 Creating SSL setup script..."
# Create SSL setup script with substituted variables
sed -e "s/\${MAIN_DOMAIN}/$MAIN_DOMAIN/g" \
    -e "s/\${COURSE_DOMAIN}/$COURSE_DOMAIN/g" \
    -e "s/\${EMAIL}/$EMAIL/g" \
    deploy/setup-ssl.sh > /root/setup_ssl.sh
chmod +x /root/setup_ssl.sh

echo "✅ Server setup complete!"
echo ""
echo "📋 Current status:"
echo "   ✅ Server configured for $MAIN_DOMAIN"
echo "   ✅ Docker installed"
echo "   ✅ nginx configured"
echo "   ✅ bytegrader user created"
echo "   ✅ Firewall enabled"
echo "   ✅ Deployment files copied"
echo ""
echo "🔔 IMPORTANT: Update your DNS now:"
echo "   A record: $COURSE_SUBDOMAIN → $(curl -s http://checkip.amazonaws.com/)"
echo "   A record: @ → $(curl -s http://checkip.amazonaws.com/)"
echo "   A record: www → $(curl -s http://checkip.amazonaws.com/)"
echo "   Then wait 5-30 minutes for DNS propagation"
echo ""
echo "📋 Next steps:"
echo "1. Test DNS: nslookup $COURSE_DOMAIN"
echo "2. Switch to bytegrader user: su - bytegrader"
echo "3. Deploy application: cd app && ./deploy.sh"
echo "4. Test HTTP: curl http://$COURSE_DOMAIN/health"
echo "5. Setup SSL: exit && sudo /root/setup_ssl.sh"
echo "6. Test HTTPS: curl https://$COURSE_DOMAIN/health"
echo "7. Test redirect: curl https://$MAIN_DOMAIN"