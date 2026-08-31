// Path-parameter parsing at the API boundary.
//
// Postgres, not Go, was validating these. A handler that interpolated a raw
// path parameter into a query let the DRIVER reject it, which surfaced as a 500
// carrying the raw SQLSTATE text:
//
//	GET /threads/{tid}/state/not-a-bigint
//	  500 {"message":"ERROR: invalid input syntax for type bigint: \"not-a-bigint\" (SQLSTATE 22P02)"}
//	GET /threads/not-a-uuid/state
//	  500 {"message":"ERROR: invalid input syntax for type uuid: \"not-a-uuid\" (SQLSTATE 22P02)"}
//
// Three things are wrong with that: a client error is reported as a server
// error, the internal schema (column names and types) leaks to the caller, and
// 500 is not in the endpoint's declared response set.
//
// STATUS CHOICE: 422, not 400 and not 404. The thread state/history/checkpoint
// paths declare exactly two responses in the OpenAPI —
// "200" Success and "422" Validation Error (duragraph-latest.yaml, the
// /threads/{thread_id}/state, /state/{checkpoint_id}, /state/checkpoint and
// /history paths). A malformed identifier IS a validation error, and returning
// an undeclared 400 would violate the contract the same way an undeclared 404
// would — the precedent set when assistants get_versions was made to return an
// empty 200 rather than a 404 the schema did not declare.
package endpoints

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// pathUUID parses a UUID-typed path parameter, 422ing a malformed one before it
// can reach a uuid column. The message names the parameter but never the
// column or the driver error.
func pathUUID(c echo.Context, name string) (uuid.UUID, error) {
	raw := c.Param(name)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid "+name+": must be a UUID")
	}
	return id, nil
}

// parseCheckpointID parses a checkpoint identifier. The OpenAPI types
// checkpoint_id as a STRING (CheckpointConfig.checkpoint_id), but a checkpoint
// is a snapshots row and snapshots.id is BIGSERIAL — so the string must denote
// a bigint. Anything else, including a value too large for int64, is a
// validation error rather than a lookup that happens to miss.
//
// Kept separate from pathUUID because the id travels two ways: as a path
// parameter (GET /threads/{id}/state/{checkpoint_id}) and inside a request body
// (POST /threads/{id}/state/checkpoint carries a CheckpointConfig), and the
// body form has no echo.Context to read from.
func parseCheckpointID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid checkpoint_id: must be a checkpoint identifier")
	}
	return id, nil
}
