-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys=OFF;
PRAGMA legacy_alter_table=ON;
BEGIN IMMEDIATE;

-- The measurement vocabulary is part of the Open Agents technical identity.
-- Rebuild the table so fresh databases enforce the canonical value directly.
-- CASE maps the one retired estimate value without naming it in this migration.
CREATE TABLE model_usage_events_next (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    binding_id                  INTEGER NOT NULL REFERENCES usage_bindings (id) ON DELETE CASCADE,
    usage_source_id             INTEGER NOT NULL REFERENCES usage_sources (id) ON DELETE CASCADE,
    provider_id                 TEXT NOT NULL CHECK (provider_id IN ('openai', 'anthropic')),
    billing_provider_id         TEXT CHECK (billing_provider_id IS NULL OR trim(billing_provider_id) <> ''),
    model_id                    TEXT NOT NULL CHECK (trim(model_id) <> ''),
    usage_measurement_kind      TEXT NOT NULL
        CHECK (usage_measurement_kind IN ('native_reported', 'open_agents_estimated', 'mixed', 'unknown')),
    input_tokens                INTEGER CHECK (input_tokens IS NULL OR input_tokens >= 0),
    cached_input_tokens         INTEGER CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
    uncached_input_tokens       INTEGER CHECK (uncached_input_tokens IS NULL OR uncached_input_tokens >= 0),
    output_tokens               INTEGER CHECK (output_tokens IS NULL OR output_tokens >= 0),
    provider_usage_json         TEXT
        CHECK (provider_usage_json IS NULL
               OR (json_valid(provider_usage_json) AND json_type(provider_usage_json, '$') = 'object')),
    source_event_key            TEXT NOT NULL CHECK (trim(source_event_key) <> ''),
    created_at                  TIMESTAMP,
    input_cost_nanos            INTEGER CHECK (input_cost_nanos IS NULL OR input_cost_nanos >= 0),
    cached_input_cost_nanos     INTEGER CHECK (cached_input_cost_nanos IS NULL OR cached_input_cost_nanos >= 0),
    output_cost_nanos           INTEGER CHECK (output_cost_nanos IS NULL OR output_cost_nanos >= 0),
    estimated_cost_nanos        INTEGER CHECK (estimated_cost_nanos IS NULL OR estimated_cost_nanos >= 0),
    pricing_version             TEXT NOT NULL DEFAULT '',
    billing_provider_source     TEXT CHECK (billing_provider_source IS NULL
                                             OR billing_provider_source IN ('observed', 'inferred')),
    UNIQUE (binding_id, source_event_key),
    CHECK (input_tokens IS NULL OR cached_input_tokens IS NULL OR uncached_input_tokens IS NULL
           OR input_tokens = cached_input_tokens + uncached_input_tokens)
);

INSERT INTO model_usage_events_next (
    id, binding_id, usage_source_id, provider_id, billing_provider_id,
    model_id, usage_measurement_kind,
    input_tokens, cached_input_tokens, uncached_input_tokens, output_tokens,
    provider_usage_json, source_event_key, created_at, input_cost_nanos,
    cached_input_cost_nanos, output_cost_nanos, estimated_cost_nanos,
    pricing_version, billing_provider_source
)
SELECT
    id, binding_id, usage_source_id, provider_id, billing_provider_id,
    model_id,
    CASE
        WHEN usage_measurement_kind IN ('native_reported', 'mixed', 'unknown')
            THEN usage_measurement_kind
        ELSE 'open_agents_estimated'
    END,
    input_tokens, cached_input_tokens, uncached_input_tokens, output_tokens,
    provider_usage_json, source_event_key, created_at, input_cost_nanos,
    cached_input_cost_nanos, output_cost_nanos, estimated_cost_nanos,
    pricing_version, billing_provider_source
FROM model_usage_events;

DROP TABLE model_usage_events;
ALTER TABLE model_usage_events_next RENAME TO model_usage_events;

CREATE INDEX idx_model_usage_events_binding_model ON model_usage_events (binding_id, model_id);
CREATE INDEX idx_model_usage_events_usage_source ON model_usage_events (usage_source_id);
CREATE INDEX idx_model_usage_events_cost_candidates
    ON model_usage_events (billing_provider_id, pricing_version, id)
    WHERE estimated_cost_nanos IS NULL;
CREATE INDEX idx_model_usage_events_canonical_cost_candidates
    ON model_usage_events (
        CASE lower(trim(billing_provider_id))
            WHEN 'z.ai' THEN 'zai'
            ELSE lower(trim(billing_provider_id))
        END,
        id
    )
    WHERE billing_provider_id IS NOT NULL
      AND estimated_cost_nanos IS NULL;
CREATE INDEX idx_model_usage_events_open_attribution
    ON model_usage_events (usage_source_id, id)
    WHERE billing_provider_id IS NULL OR billing_provider_source = 'inferred';

COMMIT;
PRAGMA legacy_alter_table=OFF;
PRAGMA foreign_keys=ON;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Additive identity cutovers do not restore a retired product vocabulary.
SELECT 1;
-- +goose StatementEnd
