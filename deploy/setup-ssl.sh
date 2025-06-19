# deploy/setup-ssl.sh - Set up SSL certificates with Let's Encrypt
# Usage: sudo ./deploy/setup-ssl.sh
# Reads configuration from ~/.bytegrader_env

set -e

# Check if we're running as root
if [ "$EUID" -ne 0 ]; then
    echo "❌ This script must be run as root"
    echo "💡 Try: sudo ./deploy/setup-ssl.sh"
    exit 1
fi

# Load environment variables from bytegrader user
BYTEGRADER_USER="${SUDO_USER:-bytegrader}"
BYTEGRADER_HOME=$(getent passwd "$BYTEGRADER_USER" | cut -d: -f6)

if [ -f "$BYTEGRADER_HOME/.bytegrader_env" ]; then
    source "$BYTEGRADER_HOME/.bytegrader_env"
    echo "✅ Loaded environment variables from $BYTEGRADER_USER user"
else
    echo "❌ Environment file not found: $BYTEGRADER_HOME/.bytegrader_env"
    echo "💡 Run server setup first: sudo ./deploy/server-setup.sh <domain> <subdomain> <email>"
    exit 1
fi

# Check that required variables are set
if [ -z "$BYTEGRADER_MAIN_DOMAIN" ] || [ -z "$BYTEGRADER_COURSE_DOMAIN" ] || [ -z "$BYTEGRADER_EMAIL" ]; then
    echo "❌ Required environment variables not set"
    echo "💡 Check $BYTEGRADER_HOME/.bytegrader_env contains:"
    echo "   BYTEGRADER_MAIN_DOMAIN"
    echo "   BYTEGRADER_COURSE_DOMAIN" 
    echo "   BYTEGRADER_EMAIL"
    exit 1
fi

echo "🔒 Setting up SSL certificates..."
echo "  Main domain: $BYTEGRADER_MAIN_DOMAIN"
echo "  Course domain: $BYTEGRADER_COURSE_DOMAIN"
echo "  Email: $BYTEGRADER_EMAIL"

# Test if DNS is working first
echo "🔍 Testing DNS resolution..."
if ! nslookup $BYTEGRADER_COURSE_DOMAIN | grep -q "$(curl -s http://checkip.amazonaws.com/)"; then
    echo "❌ DNS not yet propagated for $BYTEGRADER_COURSE_DOMAIN"
    echo "💡 Test with: nslookup $BYTEGRADER_COURSE_DOMAIN"
    echo "⏳ Wait for DNS propagation (5-30 minutes) and try again"
    exit 1
fi

echo "✅ DNS resolution working"

# Get certificates for all domains
echo "📜 Obtaining SSL certificates..."
certbot --nginx \
  -d $BYTEGRADER_MAIN_DOMAIN \
  -d www.$BYTEGRADER_MAIN_DOMAIN \
  -d $BYTEGRADER_COURSE_DOMAIN \
  --non-interactive \
  --agree-tos \
  --email $BYTEGRADER_EMAIL

# Update nginx configuration for HTTPS
echo "🔧 Updating nginx configuration for HTTPS..."
tee /etc/nginx/sites-available/bytegrader > /dev/null <<EOF
# Redirect main domain to GitHub repo (HTTPS)
server {
    listen 80;
    server_name $BYTEGRADER_MAIN_DOMAIN www.$BYTEGRADER_MAIN_DOMAIN;
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://github.com/ShawnHymel/bytegrader; }
}

server {
    listen 443 ssl http2;
    server_name $BYTEGRADER_MAIN_DOMAIN www.$BYTEGRADER_MAIN_DOMAIN;
    
    # SSL certificates (managed by certbot)
    ssl_certificate /etc/letsencrypt/live/$BYTEGRADER_MAIN_DOMAIN/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$BYTEGRADER_MAIN_DOMAIN/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;
    
    # Security headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload";
    
    location / { 
        return 301 https://github.com/ShawnHymel/bytegrader; 
    }
}

# Course API with HTTPS
server {
    listen 80;
    server_name $BYTEGRADER_COURSE_DOMAIN;
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://\$server_name\$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name $BYTEGRADER_COURSE_DOMAIN;
    
    # SSL certificates (managed by certbot)
    ssl_certificate /etc/letsencrypt/live/$BYTEGRADER_MAIN_DOMAIN/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$BYTEGRADER_MAIN_DOMAIN/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;
    
    # Security headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload";
    
    # Rate limiting
    limit_req_zone \$binary_remote_addr zone=api:10m rate=15r/m;
    
    location / {
        limit_req zone=api burst=10 nodelay;
        
        proxy_pass http://localhost:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Course-ID $BYTEGRADER_COURSE_SUBDOMAIN;
        
        proxy_read_timeout 300s;
        proxy_connect_timeout 10s;
        client_max_body_size 100M;
    }
    
    location /health {
        limit_req off;
        proxy_pass http://localhost:8080/health;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }
}
EOF

# Test nginx configuration
nginx -t

# Reload nginx
systemctl reload nginx

# Test SSL certificate renewal
echo "🔄 Testing SSL certificate renewal..."
certbot renew --dry-run

# Set up automatic renewal
echo "⏰ Setting up automatic SSL renewal..."
(crontab -l 2>/dev/null; echo "0 12 * * * /usr/bin/certbot renew --quiet") | crontab -

echo "✅ SSL setup complete!"
echo ""
echo "🌐 Your secure endpoints:"
echo "   Main site: https://$BYTEGRADER_MAIN_DOMAIN (redirects to GitHub)"
echo "   Course API: https://$BYTEGRADER_COURSE_DOMAIN"
echo ""
echo "🧪 Test your setup:"
echo "   curl https://$BYTEGRADER_COURSE_DOMAIN/health"
echo "   curl https://$BYTEGRADER_MAIN_DOMAIN (should redirect)"
echo ""
echo "📊 SSL certificate info:"
echo "   Expires: $(openssl x509 -enddate -noout -in /etc/letsencrypt/live/$BYTEGRADER_MAIN_DOMAIN/cert.pem)"
echo "   Auto-renewal: Enabled (daily check at 12:00)"
