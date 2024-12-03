BEGIN;

CREATE TABLE IF NOT EXISTS module_streams (
      uuid UUID UNIQUE NOT NULL PRIMARY KEY,
      created_at TIMESTAMP WITH TIME ZONE,
      updated_at TIMESTAMP WITH TIME ZONE,
      name text NOT NULL,
      stream text NOT NULL,
      version text NOT NULL,
      context text NOT NULL,
      arch text NOT NULL,
      summary text NOT NULL,
      description text NOT NULL,
      package_names text[] NOT NULL,
      packages text[] NOT NULL,
      hash_value text NOT NULL,
      profiles jsonb NOT NULL  DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS repositories_module_streams (
       repository_uuid UUID NOT NULL,
       module_stream_uuid UUID NOT NULL
);

CREATE INDEX IF NOT EXISTS module_streams_pkgs_idx ON module_streams USING GIN (package_names);
CREATE INDEX IF NOT EXISTS module_streams_name_idx ON module_streams (uuid, name);

ALTER TABLE ONLY repositories_module_streams
DROP CONSTRAINT IF EXISTS repositories_module_streams_pkey,
ADD CONSTRAINT repositories_module_streams_pkey PRIMARY KEY (repository_uuid, module_stream_uuid);

ALTER TABLE ONLY repositories_module_streams
DROP CONSTRAINT IF EXISTS fk_repositories_module_streams_mstream,
ADD CONSTRAINT fk_repositories_module_streams_mstream
FOREIGN KEY (module_stream_uuid) REFERENCES module_streams(uuid)
ON DELETE CASCADE;

ALTER TABLE ONLY repositories_module_streams
DROP CONSTRAINT IF EXISTS fk_repositories_module_streams_repository,
ADD CONSTRAINT fk_repositories_module_streams_repository
FOREIGN KEY (repository_uuid) REFERENCES repositories(uuid)
ON DELETE CASCADE;

ALTER TABLE ONLY module_streams
DROP CONSTRAINT IF EXISTS fk_module_streams_uniq,
ADD CONSTRAINT fk_module_streams_uniq UNIQUE (hash_value);

COMMIT;
