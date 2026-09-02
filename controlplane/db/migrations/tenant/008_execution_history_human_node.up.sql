-- Allow 'human' in execution_history.node_type.
--
-- The graph engine gained the `human` node type (graph-engine.d2 §3 human_ex,
-- "always interrupt — requires_human") but this CHECK was written before it
-- existed and still lists only start|end|llm|tool|conditional. The gap is not
-- reachable while the node is merely PAUSED — the pause writes an interrupts
-- row, not an execution_history row — so it only bites on the delivery that
-- resumes past the node and reports it completed:
--
--   POST /workers/{id}/runs/{rid}/events -> 500
--   new row for relation "execution_history" violates check constraint
--   "execution_history_node_type_check" (SQLSTATE 23514)
--
-- The worker treats that 500 as transient and Naks, so the run stalls
-- in_progress and burns every redelivery before dead-lettering to run.failed.
-- A human node is therefore unusable end to end until this lands.
--
-- The set is kept in sync with the worker's defaultExecutors (worker/graph.go):
-- start, end, conditional, human, llm, tool.
ALTER TABLE execution_history
    DROP CONSTRAINT execution_history_node_type_check;

ALTER TABLE execution_history
    ADD CONSTRAINT execution_history_node_type_check
        CHECK (node_type IN ('start', 'end', 'llm', 'tool', 'conditional', 'human'));
