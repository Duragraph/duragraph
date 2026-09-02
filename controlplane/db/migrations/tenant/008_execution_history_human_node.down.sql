-- Reverse 008_execution_history_human_node.up.sql.
--
-- NOT a pure inverse, and deliberately so: narrowing the CHECK fails outright if
-- any execution_history row already records a 'human' node, because Postgres
-- validates an added CHECK against existing rows. Those rows are real execution
-- history, so the migration refuses rather than deleting them — resolve by
-- removing or re-typing them first if this must be rolled back.
ALTER TABLE execution_history
    DROP CONSTRAINT execution_history_node_type_check;

ALTER TABLE execution_history
    ADD CONSTRAINT execution_history_node_type_check
        CHECK (node_type IN ('start', 'end', 'llm', 'tool', 'conditional'));
