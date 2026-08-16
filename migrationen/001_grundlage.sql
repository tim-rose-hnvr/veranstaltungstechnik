CREATE TABLE organisation (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name         text NOT NULL
);

CREATE TABLE saal (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organisation_id uuid NOT NULL REFERENCES organisation(id),
  name            text NOT NULL,
  UNIQUE (organisation_id, name)
);

CREATE TABLE kamera (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  saal_id  uuid NOT NULL REFERENCES saal(id),
  name     text NOT NULL,
  adresse  text NOT NULL,
  kanal    smallint NOT NULL DEFAULT 1,
  UNIQUE (saal_id, name)
);

CREATE TABLE platz (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  saal_id  uuid NOT NULL REFERENCES saal(id),
  nummer   int NOT NULL,
  name     text NOT NULL,
  UNIQUE (saal_id, nummer)
);

CREATE TABLE preset (
  id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kamera_id uuid NOT NULL REFERENCES kamera(id),
  platz_id  uuid NOT NULL REFERENCES platz(id),
  nummer    smallint NOT NULL,
  UNIQUE (kamera_id, platz_id)
);

CREATE TABLE ereignis (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  saal_id         uuid NOT NULL REFERENCES saal(id),
  folge_nr        bigint NOT NULL,
  zeit            timestamptz NOT NULL DEFAULT now(),
  art             text NOT NULL,
  nutzlast        jsonb NOT NULL DEFAULT '{}',
  vorgaenger_hash bytea,
  hash            bytea NOT NULL,
  UNIQUE (saal_id, folge_nr)
);
