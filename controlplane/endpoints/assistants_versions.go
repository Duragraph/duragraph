// Assistant version history — the write-path half (snapshotAssistant) lands in
// PR-A; the read/set_latest handlers (get_versions, set_latest) follow in PR-B.
//
// LangGraph-Cloud assistant versioning: the live assistants row holds the
// currently ACTIVE version; every create and update appends an immutable
// snapshot of the assistant's editable state to assistant_versions, keyed by
// (assistant_id, version). set_latest re-points the live row to an older
// snapshot without minting a new version, so the "next" version an update mints
// is MAX(assistant_versions.version)+1, not live.version+1 — see
// assistantNextVersionExpr.
package endpoints

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// assistantVersionColumns is the editable-state column set copied between the
// live assistants row and an assistant_versions snapshot, kept in one place so
// the snapshot INSERT and (PR-B) the set_latest copy-back stay in lockstep.
const assistantVersionColumns = `graph_id, name, description, model, instructions, tools, config, context, metadata`

// assistantNextVersionExpr is the scalar subquery that yields the next version
// number to mint for an assistant: one past the highest version ever recorded
// in its history (COALESCE handles the impossible "no snapshots yet" case as 1).
// Using the history MAX rather than the live assistants.version keeps update
// correct after a set_latest rollback re-points the live version backwards.
const assistantNextVersionExpr = `(SELECT COALESCE(MAX(version), 0) + 1 FROM assistant_versions WHERE assistant_id = $1)`

// snapshotAssistant appends an immutable snapshot of the assistant's current
// editable state to assistant_versions at its current live version. Called
// inside the same write TX as the create/update that produced that state, so
// the snapshot and the live row commit atomically. Idempotent under redelivery:
// re-snapshotting an existing (assistant_id, version) overwrites it rather than
// erroring, which cannot lose data because the source is the just-written live
// row.
func snapshotAssistant(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO assistant_versions (assistant_id, version, `+assistantVersionColumns+`)
		SELECT id, version, `+assistantVersionColumns+`
		FROM assistants WHERE id = $1
		ON CONFLICT (assistant_id, version) DO UPDATE SET
			graph_id = EXCLUDED.graph_id,
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			model = EXCLUDED.model,
			instructions = EXCLUDED.instructions,
			tools = EXCLUDED.tools,
			config = EXCLUDED.config,
			context = EXCLUDED.context,
			metadata = EXCLUDED.metadata`, id)
	return err
}
