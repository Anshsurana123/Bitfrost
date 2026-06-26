<p align="center">
  <img src="docs/bifrost-banner.png" alt="Bifrost Banner" width="800"/>
</p>

<h1 align="center">⚡ BIFRÖST — B2B AI Gateway</h1>

<p align="center">
  <strong>A Zero-Trust, Multi-Tenant AI Reverse Proxy with Semantic Caching, Real-Time Cost Analytics, and Prompt Injection Defense</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/Next.js-000000?style=for-the-badge&logo=nextdotjs&logoColor=white" />
  <img src="https://img.shields.io/badge/Supabase-3ECF8E?style=for-the-badge&logo=supabase&logoColor=white" />
  <img src="https://img.shields.io/badge/Render-46E3B7?style=for-the-badge&logo=render&logoColor=white" />
  <img src="https://img.shields.io/badge/Vercel-000000?style=for-the-badge&logo=vercel&logoColor=white" />
</p>

---

## 🧬 What is Bifröst?

Bifröst is a **production-grade, cloud-native AI API Gateway** built for B2B SaaS companies. It sits between your application and any AI provider (Gemini, OpenAI, Anthropic) and delivers:

- 🔐 **Zero-Trust Security** — HMAC-SHA256 identity fingerprinting with replay attack protection
- 🧠 **Semantic Brain** — pgvector-powered intelligent caching that recognizes _similar_ prompts, not just identical ones
- 💰 **Real-Time Cost Savings** — Automatically calculates exact Input + Output token costs saved per cache hit, across providers
- 🛡️ **Prompt Injection Auditor** — Background AI-powered security scanning via Ollama Cloud
- 🏢 **Multi-Tenant Isolation** — Complete data, cache, and key isolation per company
- 📊 **Live Telemetry Dashboard** — WebSocket-powered real-time latency, throughput, and savings monitoring

---

## 🏗️ Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                      BIFRÖST GATEWAY                         │
│                                                              │
│  ┌─────────┐    ┌──────────────┐    ┌────────────────────┐  │
│  │ Identity │───▶│  Semantic    │───▶│  Reverse Proxy     │  │
│  │ Verify   │    │  Brain       │    │  (Key Translation) │  │
│  │ (HMAC)   │    │  (pgvector)  │    │                    │  │
│  └─────────┘    └──────────────┘    └────────┬───────────┘  │
│       │                │                      │              │
│       │          ┌─────▼─────┐         ┌──────▼──────┐      │
│       │          │ Supabase  │         │ AI Provider │      │
│       │          │ PostgreSQL│         │ (Gemini/    │      │
│       │          │ + pgvector│         │  OpenAI)    │      │
│       │          └───────────┘         └─────────────┘      │
│       │                                                      │
│  ┌────▼─────────────────────────────────────────────────┐   │
│  │          Background Audit Pipeline                    │   │
│  │     (Ollama Cloud — Prompt Injection Detection)       │   │
│  └───────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│                   COMMAND DASHBOARD (Next.js)                 │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │ Live     │  │ Cost     │  │ Key      │  │ Security   │  │
│  │ Latency  │  │ Savings  │  │ Vault    │  │ Fingerprint│  │
│  │ Graph    │  │ Counter  │  │ Manager  │  │ Log        │  │
│  └──────────┘  └──────────┘  └──────────┘  └────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

---

## 📦 Repository Structure

This project consists of two deployable services:

| Service | Tech | Deployment | Repo |
|---------|------|------------|------|
| **Backend (Data Plane)** | Go 1.21+ | [Render](https://render.com) | [`bifrost-backend`](https://github.com/Anshsurana123/bifrost-backend) |
| **Dashboard (Control Plane)** | Next.js 15 + TypeScript | [Vercel](https://vercel.com) | [`bifrost-dashboard`](https://github.com/Anshsurana123/bifrost-dashboard) |

---

## 🚀 Complete Setup Guide

### Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- [Node.js 18+](https://nodejs.org/)
- A [Supabase](https://supabase.com) account (free tier works)
- A [Google AI Studio](https://aistudio.google.com/apikey) API key (for Gemini)
- An [Ollama Cloud](https://ollama.com/settings/keys) API key (for security auditing)
- A [Render](https://render.com) account (for backend hosting)
- A [Vercel](https://vercel.com) account (for dashboard hosting)

---

### Step 1: Set Up Supabase Database

1. Create a new project at [supabase.com](https://supabase.com)
2. Go to **SQL Editor** → **New Query** and run:

```sql
-- Enable vector extension for Semantic Brain
CREATE EXTENSION IF NOT EXISTS vector;

-- Persistent cache table with 3072-dim vectors (Gemini Embedding 001)
CREATE TABLE IF NOT EXISTS bifrost_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id UUID NOT NULL,
  prompt_text TEXT,
  prompt_hash TEXT NOT NULL,
  embedding vector(3072),
  response TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- Persistent key storage
CREATE TABLE IF NOT EXISTS bifrost_keys (
  virtual_key TEXT PRIMARY KEY,
  company_id UUID NOT NULL,
  real_key TEXT NOT NULL,
  app_secret TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- Row Level Security for key isolation
ALTER TABLE bifrost_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY "Users can view their own keys" ON bifrost_keys
  FOR SELECT USING (auth.uid() = company_id);

-- Semantic similarity search function
CREATE OR REPLACE FUNCTION match_prompts (
  query_embedding vector(3072),
  match_threshold float,
  match_count int,
  target_company_id uuid
)
RETURNS TABLE (
  id uuid,
  prompt_text text,
  response text,
  similarity float
)
LANGUAGE sql STABLE
AS $$
  SELECT
    id,
    prompt_text,
    response,
    1 - (embedding <=> query_embedding) AS similarity
  FROM bifrost_cache
  WHERE company_id = target_company_id
    AND 1 - (embedding <=> query_embedding) > match_threshold
  ORDER BY embedding <=> query_embedding
  LIMIT match_count;
$$;
```

3. Go to **Settings** → **API** and note down:
   - `Project URL` (e.g., `https://xxxxx.supabase.co`)
   - `anon` public key
   - `service_role` secret key

4. Go to **Authentication** → **Providers** → Enable **Google** OAuth (optional, for Google Sign-In).

---

### Step 2: Deploy the Backend on Render

1. Fork or clone [`bifrost-backend`](https://github.com/Anshsurana123/bifrost-backend)
2. Create a **New Web Service** on [Render](https://render.com)
3. Connect your GitHub repo
4. Set the following:
   - **Build Command:** `go build -o proxy`
   - **Start Command:** `./proxy`
5. Add these **Environment Variables**:

| Variable | Description | Example |
|----------|-------------|---------|
| `GEMINI_API_KEY` | Google AI Studio key for Semantic Brain embeddings | `AIza...` |
| `OLLAMA_API_KEY` | Ollama Cloud key for prompt injection auditing | `6a94bc...` |
| `SUPABASE_URL` | Your Supabase project URL | `https://xxxxx.supabase.co` |
| `SUPABASE_SERVICE_ROLE_KEY` | Supabase service role secret | `eyJhbG...` |

> ⚠️ `PORT` is automatically injected by Render. Do not set it manually.

6. Click **Deploy**. Your backend will be live at `https://your-service.onrender.com`.

---

### Step 3: Deploy the Dashboard on Vercel

1. Fork or clone [`bifrost-dashboard`](https://github.com/Anshsurana123/bifrost-dashboard)
2. Import the repo on [Vercel](https://vercel.com)
3. Add these **Environment Variables**:

| Variable | Description | Example |
|----------|-------------|---------|
| `NEXT_PUBLIC_SUPABASE_URL` | Your Supabase project URL | `https://xxxxx.supabase.co` |
| `NEXT_PUBLIC_SUPABASE_ANON_KEY` | Supabase `anon` public key | `eyJhbG...` |
| `NEXT_PUBLIC_PROXY_URL` | Your Render backend URL | `https://your-service.onrender.com` |

4. Click **Deploy**. Your dashboard will be live.

---

### Step 4: Generate Your First Virtual Key

1. Open your deployed dashboard
2. Sign up / Sign in (Email or Google OAuth)
3. Navigate to the **Key Vault** tab
4. Enter your real AI provider API key (e.g., your Gemini API key)
5. Click **Forge Virtual Key**
6. Copy the `X-Bifrost-Key` and `App Secret`

---

### Step 5: Send Requests Through the Proxy

Use the virtual key to route requests through Bifröst. Example using cURL:

```bash
# Generate HMAC fingerprint (use your app secret)
DEVICE_ID="my-device-001"
TIMESTAMP=$(date +%s)
APP_SECRET="sec-your-app-secret-here"
BIFROST_KEY="bf-vk-your-virtual-key-here"

MESSAGE="${DEVICE_ID}${APP_SECRET}${TIMESTAMP}"
FINGERPRINT=$(echo -n "$MESSAGE" | openssl dgst -sha256 -hmac "$APP_SECRET" | awk '{print $2}')

# Send request through Bifröst
curl -X POST "https://your-service.onrender.com/v1beta/models/gemini-2.0-flash:generateContent" \
  -H "Content-Type: application/json" \
  -H "X-Bifrost-Key: $BIFROST_KEY" \
  -H "X-Device-ID: $DEVICE_ID" \
  -H "X-Timestamp: $TIMESTAMP" \
  -H "X-Device-Fingerprint: $FINGERPRINT" \
  -d '{
    "contents": [{"parts": [{"text": "What is quantum computing?"}]}]
  }'
```

> 💡 **Send the same prompt twice** — the second request will be served from the Semantic Brain cache at zero cost!

---

## 🔒 Security Model

### Zero-Trust Identity Verification
Every request must include:
- `X-Bifrost-Key` — The virtual key generated from the dashboard
- `X-Device-ID` — A unique client identifier
- `X-Timestamp` — Current Unix timestamp (must be within ±30 seconds)
- `X-Device-Fingerprint` — `HMAC-SHA256(deviceId + appSecret + timestamp, appSecret)`

### Prompt Injection Defense
A background audit pipeline randomly samples incoming prompts and sends them to an Ollama Cloud LLM to detect prompt injection attacks. Malicious patterns trigger trust score degradation and eventual quarantine.

### Tenant Isolation
All cached data, API keys, and embeddings are strictly isolated by `company_id`. Company A's cached responses are **never** visible to Company B.

---

## 💰 Universal Cost Calculation

Bifröst automatically detects the AI provider from the response JSON structure and calculates precise savings:

| Provider | Input Rate | Output Rate | Detection Key |
|----------|-----------|-------------|---------------|
| **Gemini** | $0.075/1M tokens | $0.30/1M tokens | `usageMetadata.promptTokenCount` |
| **OpenAI** | $5.00/1M tokens | $15.00/1M tokens | `usage.prompt_tokens` |

When a cached response is served, both Input and Output token costs are summed to display the total savings on the dashboard.

---

## 📊 Live Dashboard Features

| Feature | Description |
|---------|-------------|
| **The Pulse** | Real-time latency graph (WebSocket-powered, updates every second) |
| **Total Savings** | Accumulated cost savings from cache hits |
| **Key Vault** | Generate, view, and copy virtual keys (persisted across sessions) |
| **Semantic Brain Toggle** | Enable/disable intelligent caching per tenant |
| **Security Fingerprint Log** | Live feed of identity verification events |
| **MCP Protocol** | Model Context Protocol endpoint for AI-native integrations |

---

## 🛠️ Local Development

### Backend
```bash
cd backend
cp .env.example .env  # Fill in your keys
go run main.go
```

### Dashboard
```bash
cd dashboard
cp .env.example .env.local  # Fill in your keys
npm install
npm run dev
```

---

## 📄 License

This project is licensed under the **[PolyForm Noncommercial License 1.0.0](LICENSE)**.

- ✅ **Free** for personal, educational, research, and non-commercial use
- 💼 **Commercial use requires a paid license** — contact [Ansh Surana](https://github.com/Anshsurana123) for commercial licensing

---

<p align="center">
  <strong>Built with 🔥 by <a href="https://github.com/Anshsurana123">Ansh Surana</a></strong>
</p>
