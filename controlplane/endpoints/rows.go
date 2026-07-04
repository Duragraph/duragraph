// Hand-written DB row structs + mappers (one set per table, reused by every
// handler that touches that table). Row structs carry `db` tags matching the
// postgres.d2 columns so pgx.RowToStructByName can scan into them; the toAPI
// mappers bridge to the oapi-codegen response types.
//
// NOTE: the OpenAPI response types and the postgres.d2 schema diverge for some
// resources (see DIVERGENCES at the bottom). Mappers map what corresponds and
// leave the rest zero/nil until the API spec ↔ postgres.d2 are reconciled.
package endpoints

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// assistantRow mirrors the assistants table (postgres.d2 workflow_ctx).
type assistantRow struct {
	ID           uuid.UUID `db:"id"`
	GraphID      *string   `db:"graph_id"`
	Name         string    `db:"name"`
	Description  *string   `db:"description"`
	Model        *string   `db:"model"`
	Instructions *string   `db:"instructions"`
	Tools        []byte    `db:"tools"`    // jsonb
	Config       []byte    `db:"config"`   // jsonb
	Metadata     []byte    `db:"metadata"` // jsonb
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// toAPI maps a row to the OpenAPI Assistant response type. config/metadata are
// jsonb in the DB; the API exposes config as a structured object and metadata
// as a free map — both are best-effort unmarshalled here.
func (r assistantRow) toAPI() Assistant {
	a := Assistant{
		AssistantId: r.ID,
		Name:        &r.Name,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		Metadata:    map[string]interface{}{},
	}
	if r.GraphID != nil {
		a.GraphId = *r.GraphID
	}
	if len(r.Config) > 0 {
		_ = json.Unmarshal(r.Config, &a.Config)
	}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &a.Metadata)
	}
	return a
}

// threadRow mirrors the threads table (postgres.d2 workflow_ctx).
type threadRow struct {
	ID        uuid.UUID `db:"id"`
	Status    string    `db:"status"`
	Values    []byte    `db:"values"`   // jsonb, nullable
	Config    []byte    `db:"config"`   // jsonb
	Metadata  []byte    `db:"metadata"` // jsonb
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// toAPI maps a row to the OpenAPI Thread response type. values/config/metadata
// are jsonb in the DB; the API exposes values/config as optional objects and
// metadata as a free map — best-effort unmarshalled here. interrupts is not
// stored on the thread row (derived from active runs); left nil.
func (r threadRow) toAPI() Thread {
	t := Thread{
		ThreadId:  r.ID,
		Status:    ThreadStatus(r.Status),
		Metadata:  map[string]interface{}{},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if len(r.Values) > 0 {
		var v map[string]interface{}
		_ = json.Unmarshal(r.Values, &v)
		t.Values = &v
	}
	if len(r.Config) > 0 {
		var c map[string]interface{}
		_ = json.Unmarshal(r.Config, &c)
		t.Config = &c
	}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &t.Metadata)
	}
	return t
}

// DIVERGENCES (OpenAPI ↔ postgres.d2) — reconcile before tightening mappers:
//   assistants: API has {config(structured), context, version}; DB has
//     {tools, model, instructions, config(jsonb)}. version/context not in DB;
//     tools/model/instructions not in the API Assistant response.
//   threads: API Thread.interrupts is derived from active runs (not a column);
//     ThreadCreate.{thread_id, if_exists, supersteps, ttl} not yet honored by
//     the create impl (always mints a fresh id). ThreadPatch.ttl not honored
//     by the update impl (metadata merge only).
