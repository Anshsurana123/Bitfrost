-- ============================================
-- BIFRÖST B2B AI GATEWAY — Supabase Setup Script
-- ============================================
-- Run this entire script in your Supabase SQL Editor
-- to initialize the persistent storage layer.
-- ============================================

-- 1. Enable the pgvector extension for Semantic Brain
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. Create the persistent cache table
--    Uses 3072-dimensional vectors (Gemini Embedding 001)
CREATE TABLE IF NOT EXISTS bifrost_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id UUID NOT NULL,
  prompt_text TEXT,
  prompt_hash TEXT NOT NULL,
  embedding vector(3072),
  response TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- 3. Create the persistent key storage table
CREATE TABLE IF NOT EXISTS bifrost_keys (
  virtual_key TEXT PRIMARY KEY,
  company_id UUID NOT NULL,
  real_key TEXT NOT NULL,
  app_secret TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- 4. Enable Row Level Security for key and cache isolation
--    This ensures tenants can only access their own keys and cache from the dashboard
ALTER TABLE bifrost_keys ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "Users can view their own keys" ON bifrost_keys;
CREATE POLICY "Users can view their own keys" ON bifrost_keys
  FOR SELECT USING (auth.uid() = company_id);

DROP POLICY IF EXISTS "Users can insert their own keys" ON bifrost_keys;
CREATE POLICY "Users can insert their own keys" ON bifrost_keys
  FOR INSERT WITH CHECK (auth.uid() = company_id);

DROP POLICY IF EXISTS "Users can update their own keys" ON bifrost_keys;
CREATE POLICY "Users can update their own keys" ON bifrost_keys
  FOR UPDATE USING (auth.uid() = company_id);

ALTER TABLE bifrost_cache ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "Users can view their own cache entries" ON bifrost_cache;
CREATE POLICY "Users can view their own cache entries" ON bifrost_cache
  FOR SELECT USING (auth.uid() = company_id);

DROP POLICY IF EXISTS "Users can insert their own cache entries" ON bifrost_cache;
CREATE POLICY "Users can insert their own cache entries" ON bifrost_cache
  FOR INSERT WITH CHECK (auth.uid() = company_id);

-- 5. Create the Semantic Similarity Search function
--    Used by the Go backend to find semantically similar cached prompts
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

-- ============================================
-- ✅ Setup Complete!
-- You can now deploy the backend and dashboard.
-- ============================================
