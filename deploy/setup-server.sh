#!/bin/bash
# deploy/setup-server.sh - Set up ByteGrader server infrastructure
# Usage: sudo ./deploy/setup-server.sh
# Run this as root from the cloned ByteGrader repository

set -e

# Check if we're running as root
if [ "$EUID" -ne 0 ]; then
    echo "❌ This script must be run as root"
    echo "💡 Try: sudo ./deploy/setup-server.sh"
    exit 1
fi

# Get the current user who ran sudo (the bytegrader user)
BYTEGRADER_USER="${SUDO_USER:-bytegrader}"

echo "🚀 ByteGrader Server Setup"
echo "=========================="
echo ""
echo "This script will configure your ByteGrader server with the following:"
echo "- Main domain (redirects to GitHub)"
echo "- Course subdomain (API endpoint)"
echo "- SSL email for certificates"
echo "- Optional IP whitelist"
echo "- Optional API key"
echo ""

# Prompt for main domain
read -p "Main domain (e.g., bytegrader.com): " MAIN_DOMAIN
if [ -z "$MAIN_DOMAIN" ]; then
    echo "❌ Main domain is required"
    exit 1
fi

# Prompt for course subdomain
read -p "Course subdomain (e.g., esp32-iot): " COURSE_SUBDOMAIN
if [ -z "$COURSE_SUBDOMAIN" ]; then
    echo "❌ Course subdomain is required"
    exit 1
fi

# Construct course domain
COURSE_DOMAIN="${COURSE_SUBDOMAIN}.${MAIN_DOMAIN}"

# Prompt for email
read -p "Email for SSL certificates: " EMAIL
if [ -z "$EMAIL" ]; then
    echo "❌ Email is required"
    exit 1
fi

# Prompt for IP whitelist (optional)
echo ""
echo "🛡️  IP Whitelist (optional - leave empty to allow all IPs):"
echo "Enter IP addresses or CIDR blocks separated by commas"
echo "Examples: 203.0.113.5,2600:4d00:500:bf::/64 or 192.168.1.0/24"
read -p "Allowed IPs (press Enter for none): " IP_WHITELIST

# Prompt for API key (optional)
echo ""
echo "🔑 API Key (optional - leave empty for no authentication):"
echo "You can enter your own key or leave empty for no API key requirement"
read -p "API Key (press Enter for none): " API_KEY

echo ""
echo "📋 Configuration Summary:"
echo "  Main domain: $MAIN_DOMAIN → GitHub redirect"
echo "  Course API: $COURSE_DOMAIN"
echo "  SSL email: $EMAIL"
echo "  Bytegrader user: $BYTEGRADER_USER"

if [ -n "$IP_WHITELIST" ]; then
    echo "  IP Whitelist: $IP_WHITELIST"
else
    echo "  IP Whitelist: DISABLED (allow all IPs)"
fi

if [ -n "$API_KEY" ]; then
    echo "  API Key: ****** (configured)"
else
    echo "  API Key: DISABLED (no authentication)"
fi

echo ""
read -p "Continue with setup? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Setup cancelled."
    exit 1
fi

echo ""
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

# Set security configuration based on user input
if [ -n "$API_KEY" ]; then
    REQUIRE_API_KEY="true"
else
    REQUIRE_API_KEY="false"
    API_KEY=""
fi

# Create comprehensive environment file for bytegrader user
sudo -u "$BYTEGRADER_USER" tee "$BYTEGRADER_HOME/.bytegrader_env" > /dev/null <<EOF
# ByteGrader Environment Variables - Generated $(date)
# Domain Configuration
export BYTEGRADER_MAIN_DOMAIN="$MAIN_DOMAIN"
export BYTEGRADER_COURSE_SUBDOMAIN="$COURSE_SUBDOMAIN"
export BYTEGRADER_COURSE_DOMAIN="$COURSE_DOMAIN"
export BYTEGRADER_EMAIL="$EMAIL"

# Security Configuration
export BYTEGRADER_REQUIRE_API_KEY="$REQUIRE_API_KEY"
export BYTEGRADER_VALID_API_KEYS="$API_KEY"
export BYTEGRADER_ALLOWED_IPS="$IP_WHITELIST"

# Application Settings (can be overridden in docker-compose.yml)
export BYTEGRADER_MAX_FILE_SIZE_MB="50"
export BYTEGRADER_GRADING_TIMEOUT_MIN="10"
export BYTEGRADER_MAX_CONCURRENT_JOBS="1"
export BYTEGRADER_RATE_LIMIT_ENABLED="true"
export BYTEGRADER_RATE_LIMIT_REQUESTS="100"
export BYTEGRADER_RATE_LIMIT_WINDOW_MIN="5"

# Cleanup Settings
export BYTEGRADER_CLEANUP_INTERVAL_HOURS="1"
export BYTEGRADER_COMPLETED_JOB_TTL_HOURS="24"
export BYTEGRADER_FAILED_JOB_TTL_HOURS="24"
export BYTEGRADER_OLD_FILE_TTL_HOURS="48"
EOF

# Add to .bashrc if not already there
if ! grep -q "source ~/.bytegrader_env" "$BYTEGRADER_HOME/.bashrc" 2>/dev/null; then
    sudo -u "$BYTEGRADER_USER" bash -c 'echo "source ~/.bytegrader_env" >> ~/.bashrc'
fi

echo "✅ Environment variables configured"

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

echo "✅ Server setup complete!"
echo ""
echo "📋 Configuration Summary:"
echo "   Main domain: $MAIN_DOMAIN → GitHub redirect"
echo "   Course API: $COURSE_DOMAIN"
echo "   User: $BYTEGRADER_USER"

if [ -n "$IP_WHITELIST" ]; then
    echo "   🛡️  IP Whitelist: $IP_WHITELIST"
else
    echo "   🌐 IP Access: Open (no restrictions)"
fi

if [ -n "$API_KEY" ]; then
    echo "   🔑 API Key: Required (configured)"
    echo ""
    echo "🔑 Your API Key: $API_KEY"
    echo "💾 Save this key securely! You'll need it for API clients."
    echo ""
    echo "📝 API Usage Examples:"
    echo "   curl -H \"X-API-Key: $API_KEY\" https://$COURSE_DOMAIN/health"
    echo "   curl -H \"Authorization: Bearer $API_KEY\" https://$COURSE_DOMAIN/health"
else
    echo "   🔓 API Key: Not required"
fi

echo ""
echo "🔔 Next: Update DNS records:"
echo "   A    $COURSE_SUBDOMAIN    $(curl -s http://checkip.amazonaws.com/)"
echo "   A    @                    $(curl -s http://checkip.amazonaws.com/)"
echo "   A    www                  $(curl -s http://checkip.amazonaws.com/)"
echo ""
echo "📋 Then deploy application:"
echo "   exit                           # Back to $BYTEGRADER_USER user"
echo "   ./deploy/deploy.sh ~/app       # Deploy application"
echo "   sudo ./deploy/setup-ssl.sh     # Enable HTTPS (after DNS propagates)"
echo ""
echo "🔐 Configuration saved to: $BYTEGRADER_HOME/.bytegrader_env"
