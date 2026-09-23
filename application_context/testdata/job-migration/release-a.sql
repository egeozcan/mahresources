-- Release A source tables before the Task 11 Job backfill. These intentionally
-- contain no Job source mapping or migration checkpoint tables; startup adds
-- those separately after upgrading the application schema.
CREATE TABLE download_history_entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id TEXT NOT NULL,
  url TEXT,
  name TEXT,
  status TEXT NOT NULL,
  error TEXT,
  resource_id INTEGER,
  total_size INTEGER,
  progress INTEGER,
  attempts INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME,
  started_at DATETIME,
  completed_at DATETIME,
  created_by_user_id INTEGER,
  plugin_name TEXT,
  payload JSON,
  last_retry_job_id TEXT,
  last_retry_at DATETIME
);

CREATE TABLE plugin_command_runs (
  id TEXT PRIMARY KEY,
  job_id TEXT,
  job_execution_token TEXT,
  plugin_name TEXT NOT NULL,
  command_name TEXT NOT NULL,
  params_json TEXT NOT NULL,
  inputs_json TEXT,
  status TEXT NOT NULL,
  exit_code INTEGER,
  error TEXT,
  process_group_id INTEGER,
  boot_session_id TEXT,
  cancel_requested NUMERIC,
  output_unverified NUMERIC,
  actorless_at_submission NUMERIC,
  created_by_user_id INTEGER,
  created_at DATETIME,
  started_at DATETIME,
  finished_at DATETIME,
  exchange_removed_at DATETIME
);

CREATE TABLE plugin_command_imports (
  id TEXT PRIMARY KEY,
  job_id TEXT,
  job_execution_token TEXT,
  run_id TEXT NOT NULL,
  file_name TEXT NOT NULL,
  fields_json TEXT,
  plugin_generation INTEGER,
  created_by_user_id INTEGER,
  status TEXT NOT NULL,
  error TEXT,
  source_delete_pending NUMERIC,
  created_at DATETIME,
  started_at DATETIME,
  finished_at DATETIME
);

INSERT INTO download_history_entries
  (job_id, url, name, status, error, attempts, created_at, completed_at, payload)
VALUES
  ('release-a-download', 'https://user:secret@old.example/file?sig=private', 'old asset', 'failed', 'fetch failed', 1,
   '2025-01-02 03:04:05', '2025-01-02 03:05:05',
   '{"url":"https://user:secret@old.example/file?sig=private","headers":{"Authorization":"Bearer old-secret"}}');

INSERT INTO plugin_command_runs
  (id, plugin_name, command_name, params_json, inputs_json, status, created_by_user_id, created_at, started_at, finished_at)
VALUES
  ('release-a-run', 'worker', 'import', '{"page":3}', '[]', 'succeeded', 7,
   '2025-01-02 03:04:05', '2025-01-02 03:04:10', '2025-01-02 03:05:05');

INSERT INTO plugin_command_imports
  (id, run_id, file_name, fields_json, plugin_generation, created_by_user_id, status, created_at)
VALUES
  ('release-a-import', 'release-a-run', 'old.csv', '{"title":"from old schema"}', 1, 7, 'pending',
   '2025-01-02 03:06:05');
