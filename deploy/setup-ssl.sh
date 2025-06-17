#!/bin/bash
# scripts/setup-ssl.sh - Set up SSL certificates with Let's Encrypt
# This is a template file - variables will be substituted by server-setup.sh
# Variables: ${MAIN_DOMAIN}, ${COURSE_DOMAIN}, ${EMAIL}

set -e

MAIN_DOMAIN="${MAIN_DOMAIN}"
COURSE_DOMAIN="${COURSE_DOMAIN}"
EMAIL="${EMAIL}"

echo "🔒 Setting up SSL certificates..."
echo "  Main domain: $MAIN_DOMAIN"
echo "  Course domain: $COURSE_DOMAIN"
echo "  Email: $EMAIL"

# Test if DNS is working first
echo "🔍 Testing DNS resolution..."
if ! nslookup $COURSE_DOMAIN | grep -q "$(curl -s http://checkip.amazonaws.com/)"; then
    echo "❌ DNS not yet propagated for $COURSE_DOMAIN"
    echo "💡 Test with: nslookup $COURSE_DOMAIN"
    echo "⏳ Wait for DNS propagation (5-30 minutes) and try again"
    exit 1
fi

echo "✅ DNS resolution working"

# Get certificates for all domains
echo "📜 Obtaining SSL certificates..."
certbot --nginx \
  -d $MAIN_DOMAIN \
  -d www.$MAIN_DOMAIN \
  -d $COURSE_DOMAIN \
  --non-interactive \
  --agree-tos \
  --email $EMAIL

# Update nginx configuration for HTTPS
echo "🔧 Updating nginx configuration for HTTPS..."
tee /etc/nginx/sites-available/bytegrader > /dev/null <<EOF
# Redirect main domain to GitHub repo (HTTPS)
server {
    listen 80;
    server_name $MAIN_DOMAIN www.$MAIN_DOMAIN;
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://github.com/ShawnHymel/bytegrader; }
}

server {
    listen 443 ssl http2;
    server_name $MAIN_DOMAIN www.$MAIN_DOMAIN;
    
    # SSL certificates (managed by certbot)
    ssl_certificate /etc/letsencrypt/live/$MAIN_DOMAIN/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$MAIN_DOMAIN/privkey.pem;
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
    server_name $COURSE_DOMAIN;
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://\$server_name\$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name $COURSE_DOMAIN;
    
    # SSL certificates (managed by certbot)
    ssl_certificate /etc/letsencrypt/live/$MAIN_DOMAIN/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$MAIN_DOMAIN/privkey.pem;
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
        proxy_set_header X-Course-ID ${COURSE_SUBDOMAIN};
        
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
echo "   Main site: https://$MAIN_DOMAIN (redirects to GitHub)"
echo "   Course API: https://$COURSE_DOMAIN"
echo ""
echo "🧪 Test your setup:"
echo "   curl https://$COURSE_DOMAIN/health"
echo "   curl https://$MAIN_DOMAIN (should redirect)"
echo ""
echo "📊 SSL certificate info:"
echo "   Expires: $(openssl x509 -enddate -noout -in /etc/letsencrypt/live/$MAIN_DOMAIN/cert.pem)"
echo "   Auto-renewal: Enabled (daily check at 12:00)"