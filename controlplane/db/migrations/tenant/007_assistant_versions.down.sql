-- Reverse 007_assistant_versions.up.sql. The table is pure history derived from
-- the live assistants rows; dropping it loses version history but not any live
-- assistant (assistants.version remains the active version).
DROP TABLE IF EXISTS assistant_versions;
