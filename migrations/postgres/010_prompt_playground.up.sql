CREATE TABLE prompt_playground_sessions (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  source_span_id  TEXT,
  source_run_id   TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_playground_sessions_project ON prompt_playground_sessions (project_id, created_at DESC);

CREATE TABLE prompt_playground_variants (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id   UUID NOT NULL REFERENCES prompt_playground_sessions(id) ON DELETE CASCADE,
  label        TEXT NOT NULL,
  model_id     TEXT NOT NULL,
  system_prompt TEXT,
  messages     JSONB NOT NULL,
  temperature  REAL,
  max_tokens   INTEGER,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_playground_variants_session ON prompt_playground_variants (session_id);

CREATE TABLE prompt_playground_executions (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  variant_id     UUID NOT NULL REFERENCES prompt_playground_variants(id) ON DELETE CASCADE,
  output         TEXT,
  input_tokens   INTEGER NOT NULL DEFAULT 0,
  output_tokens  INTEGER NOT NULL DEFAULT 0,
  cost_usd       DOUBLE PRECISION NOT NULL DEFAULT 0,
  latency_ms     INTEGER NOT NULL DEFAULT 0,
  error          TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_playground_executions_variant ON prompt_playground_executions (variant_id, created_at DESC);
