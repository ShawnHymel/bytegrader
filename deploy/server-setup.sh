#!/bin/bash
# deploy/server-setup.sh - Set up ByteGrader server infrastructure
# Usage: sudo ./deploy/server-setup.sh <main_domain> <course_subdomain> <email>
# Run this as root from the cloned ByteGrader repository

set -e

# Check if we're running as root
if [ "$EUID" -ne 0 ]; then
    echo "❌ This script must be run as root"
    echo "💡 Try: sudo ./deploy/server-setup.sh <domain> <subdomain> <email>"
    exit 1
fi

# Check arguments
if [ $# -ne 3 ]; then
    echo "Usage: $0 <main_domain> <course_subdomain> <email>"
    echo ""
    echo "Examples:"
    echo "  $0 bytegrader.com esp32-iot you@example.com"
    echo "  $0 mydomain.com python101 admin@mydomain.com"
    echo ""
    exit 1
fi

# Parse arguments
MAIN_DOMAIN="$1"
COURSE_SUBDOMAIN="$2"
EMAIL="$3"
COURSE_DOMAIN="${COURSE_SUBDOMAIN}.${MAIN_DOMAIN}"

# Get the current user who ran sudo (the bytegrader user)
BYTEGRADER_USER="${SUDO_USER:-bytegrader}"

echo "🚀 Setting up ByteGrader server infrastructure:"
echo "  Main domain: $MAIN_DOMAIN (redirects to GitHub)"
echo "  Course API: $COURSE_DOMAIN"
echo "  SSL email: $EMAIL"
echo "  Bytegrader user: $BYTEGRADER_USER"
echo ""

# Confirm before proceeding
read -p "Continue? (y/N): " -n 1 -r
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
if ! command -v docker &> /dev/null; then
    curl -fsSL https://get.docker.com -o get-docker.sh
    sh get-docker.sh && rm get-docker.sh
    apt install -y docker-compose-plugin
    echo "✅ Docker installed"
else
    echo "✅ Docker already installed"
fi

echo "👤 Configuring bytegrader user..."
# Ensure user is in correct groups
usermod -aG docker "$BYTEGRADER_USER"
usermod -aG sudo "$BYTEGRADER_USER"

echo "🔧 Setting up environment variables..."
BYTEGRADER_HOME=$(getent passwd "$BYTEGRADER_USER" | cut -d: -f6)

# Create environment file for bytegrader user
sudo -u "$BYTEGRADER_USER" tee "$BYTEGRADER_HOME/.bytegrader_env" > /dev/null <<EOF
# ByteGrader Environment Variables - Generated $(date)
export BYTEGRADER_MAIN_DOMAIN="$MAIN_DOMAIN"
export BYTEGRADER_COURSE_SUBDOMAIN="$COURSE_SUBDOMAIN"
export BYTEGRADER_COURSE_DOMAIN="$COURSE_DOMAIN"
export BYTEGRADER_EMAIL="$EMAIL"
EOF

# Add to .bashrc if not already there
if ! grep -q "source ~/.bytegrader_env" "$BYTEGRADER_HOME/.bashrc" 2>/dev/null; then
    sudo -u "$BYTEGRADER_USER" bash -c 'echo "source ~/.bytegrader_env" >> ~/.bashrc'
fi

echo "🌐 Setting up nginx configuration..."
# Check if nginx template exists
if [ ! -f "deploy/bytegrader.conf.template" ]; then
    echo "❌ nginx template not found at deploy/bytegrader.conf.template"
    echo "💡 Make sure you're running this from the ByteGrader repository root"
    exit 1
fi

# Process nginx template
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

echo "🔧 Setting up SSL preparation script..."
# Check if SSL template exists
if [ -f "deploy/setup-ssl.sh" ]; then
    sed -e "s/\${MAIN_DOMAIN}/$MAIN_DOMAIN/g" \
        -e "s/\${COURSE_DOMAIN}/$COURSE_DOMAIN/g" \
        -e "s/\${EMAIL}/$EMAIL/g" \
        deploy/setup-ssl.sh > /root/setup_ssl.sh
    chmod +x /root/setup_ssl.sh
    echo "✅ SSL setup script created at /root/setup_ssl.sh"
else
    echo "⚠️  SSL template not found, you'll need to configure SSL manually"
fi

echo "✅ Server setup complete!"
echo ""
echo "📋 Configuration:"
echo "   Main domain: $MAIN_DOMAIN → GitHub redirect"
echo "   Course API: $COURSE_DOMAIN"
echo "   User: $BYTEGRADER_USER"
echo ""
echo "🔔 Next: Update DNS records:"
echo "   A    $COURSE_SUBDOMAIN    $(curl -s http://checkip.amazonaws.com/)"
echo "   A    @                    $(curl -s http://checkip.amazonaws.com/)"
echo "   A    www                  $(curl -s http://checkip.amazonaws.com/)"
echo ""
echo "📋 Then deploy application:"
echo "   exit                     # Back to $BYTEGRADER_USER user"
echo "   ./deploy/deploy.sh       # Deploy application"
echo "   sudo /root/setup_ssl.sh  # Enable HTTPS (after DNS propagates)"
