# ViralClip AI — Deployment Guide

This guide covers deploying ViralClip AI to production using Docker Compose (single server) or Kubernetes (multi-node cluster).

---

## Prerequisites

- Docker ≥ 24.0 and Docker Compose ≥ 2.20
- A server with at least 4 vCPU, 8 GB RAM (16 GB recommended for Whisper `medium` model)
- Domain name with DNS pointing to your server
- OpenAI API key
- (Optional) AWS S3 bucket for video storage
- (Optional) Stripe account for subscription billing

---

## Option 1: Docker Compose (Single Server)

Suitable for small to medium workloads (up to ~500 active users).

### Step 1: Clone and Configure

```bash
git clone https://github.com/pindoyono/ViralClip-AI.git
cd ViralClip-AI

# Generate secure secrets
export JWT_SECRET=$(openssl rand -hex 32)
export DB_PASSWORD=$(openssl rand -hex 24)
```

### Step 2: Create Environment File

```bash
cat > .env << EOF
# Application
APP_ENV=production
LOG_LEVEL=info

# Database
DATABASE_NAME=viralclip
DATABASE_USER=viralclip
DATABASE_PASSWORD=${DB_PASSWORD}
DATABASE_PORT=5432

# Redis
REDIS_PASSWORD=$(openssl rand -hex 16)

# JWT
JWT_SECRET=${JWT_SECRET}
JWT_EXPIRES_IN=24h
JWT_REFRESH_EXPIRES_IN=168h

# OpenAI
OPENAI_API_KEY=sk-your-api-key-here
OPENAI_MODEL=gpt-4-turbo-preview
WHISPER_MODEL=base

# Storage (choose local or s3)
STORAGE_PROVIDER=local
LOCAL_STORAGE_PATH=/app/storage

# Frontend
NEXT_PUBLIC_API_URL=https://api.yourdomain.com
NEXT_PUBLIC_APP_URL=https://app.yourdomain.com

# Optional: Stripe
STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...

# Optional: Google OAuth
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...

# Optional: Sentry
SENTRY_DSN=https://...

# Optional: CORS
CORS_ORIGINS=https://app.yourdomain.com
EOF
```

### Step 3: Configure Nginx TLS

Update `infrastructure/nginx/nginx.conf` with your domain:

```nginx
server_name api.yourdomain.com;
ssl_certificate /etc/nginx/ssl/fullchain.pem;
ssl_certificate_key /etc/nginx/ssl/privkey.pem;
```

Place your TLS certificates in `infrastructure/nginx/ssl/`.

Using Let's Encrypt with Certbot:
```bash
certbot certonly --standalone -d api.yourdomain.com -d app.yourdomain.com
```

### Step 4: Start Services

```bash
# Pull and build all images
docker compose build

# Start in detached mode
docker compose up -d

# Monitor startup
docker compose logs -f --tail=50
```

### Step 5: Run Database Migrations

```bash
# Migrations are auto-applied on API startup via GORM AutoMigrate
# Verify by checking API logs:
docker compose logs api | grep -i "migrat"
```

### Step 6: Verify Deployment

```bash
# Health checks
curl https://api.yourdomain.com/health
curl https://app.yourdomain.com

# Check all containers are running
docker compose ps
```

### Updating to a New Version

```bash
git pull origin main
docker compose build
docker compose up -d --no-deps --build api worker ai-service web
```

---

## Option 2: Kubernetes (Production Scale)

Suitable for high availability and auto-scaling deployments.

### Prerequisites

- Kubernetes cluster ≥ 1.28 (EKS, GKE, AKS, or self-managed)
- `kubectl` configured for your cluster
- Helm ≥ 3.12
- A container registry (ECR, GCR, Docker Hub)

### Step 1: Build and Push Images

```bash
# Set your registry
export REGISTRY=your-registry.io/viralclip

# Build and push all images
docker build -t ${REGISTRY}/api:latest apps/api/
docker build -t ${REGISTRY}/worker:latest apps/worker/
docker build -t ${REGISTRY}/ai-service:latest apps/ai-service/
docker build -t ${REGISTRY}/web:latest apps/web/

docker push ${REGISTRY}/api:latest
docker push ${REGISTRY}/worker:latest
docker push ${REGISTRY}/ai-service:latest
docker push ${REGISTRY}/web:latest
```

### Step 2: Create Namespace and Secrets

```bash
kubectl create namespace viralclip

# Database credentials
kubectl create secret generic viralclip-db \
  --namespace viralclip \
  --from-literal=url="postgresql://viralclip:${DB_PASSWORD}@postgres-svc:5432/viralclip"

# JWT and API secrets
kubectl create secret generic viralclip-api \
  --namespace viralclip \
  --from-literal=jwt-secret="${JWT_SECRET}" \
  --from-literal=openai-api-key="${OPENAI_API_KEY}"

# Redis credentials
kubectl create secret generic viralclip-redis \
  --namespace viralclip \
  --from-literal=url="redis://:${REDIS_PASSWORD}@redis-svc:6379/0"
```

### Step 3: Deploy Infrastructure

```bash
# PostgreSQL (use managed RDS/Cloud SQL in production instead)
kubectl apply -f infrastructure/kubernetes/postgres.yaml -n viralclip

# Redis (use ElastiCache/Memorystore in production instead)
kubectl apply -f infrastructure/kubernetes/redis.yaml -n viralclip

# Wait for database to be ready
kubectl wait --for=condition=ready pod -l app=postgres -n viralclip --timeout=120s
```

### Step 4: Deploy Application Services

```bash
kubectl apply -f infrastructure/kubernetes/ -n viralclip

# Verify all deployments
kubectl get deployments -n viralclip
kubectl get pods -n viralclip
kubectl get services -n viralclip
```

### Step 5: Configure Ingress

```bash
# Install NGINX Ingress Controller (if not already installed)
helm upgrade --install ingress-nginx ingress-nginx \
  --repo https://kubernetes.github.io/ingress-nginx \
  --namespace ingress-nginx --create-namespace

# Apply Ingress rules
kubectl apply -f infrastructure/kubernetes/ingress.yaml -n viralclip
```

### Step 6: Set Up Horizontal Pod Autoscaler

```bash
# API: scale between 2 and 10 replicas based on CPU
kubectl autoscale deployment viralclip-api \
  --namespace viralclip \
  --min=2 --max=10 --cpu-percent=70

# Worker: scale between 1 and 5 replicas
kubectl autoscale deployment viralclip-worker \
  --namespace viralclip \
  --min=1 --max=5 --cpu-percent=80

# AI Service: scale based on GPU/CPU
kubectl autoscale deployment viralclip-ai \
  --namespace viralclip \
  --min=1 --max=4 --cpu-percent=75
```

### Useful kubectl Commands

```bash
# View logs for a service
kubectl logs -f deployment/viralclip-api -n viralclip

# Exec into a pod
kubectl exec -it deployment/viralclip-api -n viralclip -- /bin/sh

# Scale a deployment manually
kubectl scale deployment viralclip-api --replicas=3 -n viralclip

# Rolling restart
kubectl rollout restart deployment/viralclip-api -n viralclip

# Check rollout status
kubectl rollout status deployment/viralclip-api -n viralclip
```

---

## Storage Configuration

### Local Storage (Default)

Files are stored on the Docker volume `storage-data`. Suitable for single-server deployments.

```env
STORAGE_PROVIDER=local
LOCAL_STORAGE_PATH=/app/storage
```

### AWS S3

For production multi-server deployments, use S3:

```env
STORAGE_PROVIDER=s3
AWS_BUCKET=viralclip-videos
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=AKIA...
AWS_SECRET_ACCESS_KEY=...
```

Create the S3 bucket with appropriate CORS and lifecycle policies:

```bash
aws s3 mb s3://viralclip-videos --region us-east-1
aws s3api put-bucket-cors --bucket viralclip-videos --cors-configuration file://infrastructure/s3/cors.json
```

---

## Database Backups

### Automated PostgreSQL Backups

```bash
# Create a backup
docker compose exec postgres pg_dump -U viralclip viralclip | gzip > backup_$(date +%Y%m%d).sql.gz

# Restore from backup
gunzip -c backup_20240115.sql.gz | docker compose exec -T postgres psql -U viralclip viralclip
```

For production, use pg_basebackup with WAL archiving or a managed database service.

---

## Performance Tuning

### Whisper Model Selection

| Model  | VRAM  | Speed  | Accuracy | Recommended For         |
|--------|-------|--------|----------|-------------------------|
| `tiny` | 1 GB  | 32×    | Low      | Development             |
| `base` | 1 GB  | 16×    | Medium   | Small production        |
| `small`| 2 GB  | 6×     | Good     | Medium production       |
| `medium`| 5 GB | 2×     | High     | High-quality production |
| `large`| 10 GB | 1×    | Highest  | Enterprise              |

Set in `.env`:
```env
WHISPER_MODEL=medium
WHISPER_DEVICE=cuda  # for GPU acceleration
```

### PostgreSQL Tuning

For production, adjust `postgresql.conf`:
```
max_connections = 200
shared_buffers = 2GB
effective_cache_size = 6GB
work_mem = 16MB
maintenance_work_mem = 512MB
```

### Redis Memory Policy

```
maxmemory 512mb
maxmemory-policy allkeys-lru
```

---

## Health Monitoring

Set up uptime monitoring for:
- `https://api.yourdomain.com/health` → expect `{"status":"ok"}`
- `https://app.yourdomain.com` → expect HTTP 200
- `http://ai-service:8000/health` → expect `{"status":"ok"}` (internal)

Recommended: Use Grafana + Prometheus with the API's `/metrics` endpoint.

---

## SSL/TLS Renewal

Using Certbot with auto-renewal:

```bash
# Test renewal
certbot renew --dry-run

# Add to crontab for automatic renewal
0 12 * * * /usr/bin/certbot renew --quiet && docker compose exec nginx nginx -s reload
```

---

## Rollback Procedure

```bash
# Docker Compose
git checkout <previous-tag>
docker compose build
docker compose up -d

# Kubernetes
kubectl rollout undo deployment/viralclip-api -n viralclip
kubectl rollout undo deployment/viralclip-worker -n viralclip
```
