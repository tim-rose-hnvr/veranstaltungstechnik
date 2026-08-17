CREATE TABLE person (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organisation_id uuid NOT NULL REFERENCES organisation(id),
  name            text NOT NULL,
  UNIQUE (organisation_id, name)
);

CREATE TABLE sitzung (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  saal_id  uuid NOT NULL REFERENCES saal(id),
  titel    text NOT NULL,
  zustand  text NOT NULL DEFAULT 'vorbereitet',
  beginn   timestamptz,
  ende     timestamptz,
  CONSTRAINT sitzung_zustand CHECK (zustand IN
    ('vorbereitet','bereit','laufend','unterbrochen','geschlossen','archiviert'))
);

CREATE TABLE teilnahme (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sitzung_id uuid NOT NULL REFERENCES sitzung(id),
  person_id  uuid NOT NULL REFERENCES person(id),
  platz_id   uuid NOT NULL REFERENCES platz(id),
  rolle      text NOT NULL,
  zustand    text NOT NULL DEFAULT 'eingeladen',
  pin_hash   bytea NOT NULL,
  UNIQUE (sitzung_id, person_id),
  UNIQUE (sitzung_id, platz_id),
  CONSTRAINT teilnahme_rolle CHECK (rolle IN
    ('leitung','delegierter','schriftfuehrung','gast')),
  CONSTRAINT teilnahme_zustand CHECK (zustand IN
    ('eingeladen','registriert','angemeldet','anwesend','abwesend'))
);

CREATE TABLE wortmeldung (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sitzung_id   uuid NOT NULL REFERENCES sitzung(id),
  teilnahme_id uuid NOT NULL REFERENCES teilnahme(id),
  folge_nr     bigint NOT NULL,
  zustand      text NOT NULL DEFAULT 'gemeldet',
  gemeldet     timestamptz NOT NULL DEFAULT now(),
  erteilt      timestamptz,
  beendet      timestamptz,
  UNIQUE (sitzung_id, folge_nr),
  CONSTRAINT wortmeldung_zustand CHECK (zustand IN
    ('gemeldet','erteilt','laufend','beendet','entzogen','zurueckgezogen'))
);
