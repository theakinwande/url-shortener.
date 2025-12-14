# Deploy URL Shortener to Railway

Railway is the fastest way to deploy this URL shortener. You'll have it live in ~10 minutes.

## Prerequisites

1. **GitHub account** (to connect your repo)
2. **Railway account** - Sign up at [railway.app](https://railway.app)

---

## Step 1: Push Code to GitHub

First, push your code to a GitHub repository:

```bash
# Initialize git (if not already done)
git init

# Add all files
git add .

# Commit
git commit -m "Initial commit - URL Shortener"

# Create repo on GitHub, then:
git remote add origin https://github.com/theakinwande/url-shortener.git
git branch -M main
git push -u origin main
```

---

## Step 2: Create Railway Project

1. Go to [railway.app](https://railway.app) and sign in with GitHub
2. Click **"New Project"**
3. Select **"Deploy from GitHub repo"**
4. Choose your `url-shortener` repository

---

## Step 3: Add PostgreSQL

1. In your Railway project, click **"+ New"**
2. Select **"Database"** → **"PostgreSQL"**
3. Railway will automatically create a PostgreSQL instance

---

## Step 4: Add Redis

1. Click **"+ New"** again
2. Select **"Database"** → **"Redis"**
3. Railway will create a Redis instance

---

## Step 5: Configure Environment Variables

Click on your **app service** (not the databases), then go to **"Variables"**:

Add these variables:

| Variable       | Value                               |
| -------------- | ----------------------------------- |
| `PORT`         | `${{PORT}}` (Railway provides this) |
| `GIN_MODE`     | `release`                           |
| `DATABASE_URL` | `${{Postgres.DATABASE_URL}}`        |
| `REDIS_URL`    | `${{Redis.REDIS_URL}}`              |
| `BASE_URL`     | `https://YOUR-APP.up.railway.app`   |

> **Note**: The `${{Postgres.DATABASE_URL}}` syntax auto-references the database you added.

---

## Step 6: Run Database Migrations

Railway doesn't auto-run migrations. Use Railway CLI:

```bash
# Install Railway CLI
npm install -g @railway/cli

# Login
railway login

# Link to your project
railway link

# Run migrations
railway run psql $DATABASE_URL -f migrations/001_create_urls.sql
railway run psql $DATABASE_URL -f migrations/002_create_api_keys.sql
```

---

## Step 7: Deploy!

Railway auto-deploys when you push to GitHub. Check the **Deployments** tab for logs.

Your app will be live at:

```
https://YOUR-APP-NAME.up.railway.app
```

---

## Step 8: Create Your First API Key

Use Railway's CLI to connect to PostgreSQL:

```bash
railway run psql $DATABASE_URL
```

Then run:

```sql
INSERT INTO api_keys (key_hash, name, rate_limit) VALUES (
  'a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3',
  'Production Key',
  1000
);
```

This creates an API key where `123` is the secret.

---

## Step 9: Set Up Custom Domain (Optional)

1. In Railway, go to your app's **Settings**
2. Click **"Generate Domain"** or add a **Custom Domain**
3. Update `BASE_URL` environment variable to match

---

## Test Your Deployment

```bash
# Health check
curl https://YOUR-APP.up.railway.app/health

# Create a short URL
curl -X POST https://YOUR-APP.up.railway.app/api/shorten \
  -H "Content-Type: application/json" \
  -H "X-API-Key: 123" \
  -d '{"url": "https://github.com"}'
```

---

## Costs

- **Free tier**: 500 hours/month, 512MB RAM
- **Hobby plan**: $5/month for always-on apps
- **PostgreSQL**: ~$5/month
- **Redis**: ~$5/month

Total for a small production setup: **~$15/month**
