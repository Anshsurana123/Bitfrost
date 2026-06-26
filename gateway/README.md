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
