package checkpoint

import "errors"

var (
	ErrInvalidThreadID    = errors.New("thread_id is required")
	ErrCheckpointNotFound = errors.New("checkpoint not found")
)
