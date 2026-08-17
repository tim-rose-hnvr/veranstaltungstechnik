CREATE TABLE abstimmung (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sitzung_id      uuid NOT NULL REFERENCES sitzung(id),
  folge_nr        bigint NOT NULL,
  titel           text NOT NULL,
  art             text NOT NULL,
  zustand         text NOT NULL DEFAULT 'vorbereitet',
  -- Beim Start eingefroren: eine später eintreffende Person ändert die
  -- Beschlussfähigkeit einer laufenden Abstimmung nicht mehr.
  stimmberechtigt int NOT NULL DEFAULT 0,
  quorum          int NOT NULL DEFAULT 0,
  anwesend        int NOT NULL DEFAULT 0,
  beginn          timestamptz,
  ende            timestamptz,
  UNIQUE (sitzung_id, folge_nr),
  CONSTRAINT abstimmung_art CHECK (art IN ('offen','namentlich','geheim')),
  CONSTRAINT abstimmung_zustand CHECK (zustand IN
    ('vorbereitet','laufend','ausgezaehlt','festgestellt','abgebrochen'))
);

-- Bei geheimer Wahl bleibt teilnahme_id leer. Die Zuordnung Stimme zu Person
-- existiert dann nirgends — auch nicht im Ereignisprotokoll.
CREATE TABLE stimme (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  abstimmung_id uuid NOT NULL REFERENCES abstimmung(id),
  teilnahme_id  uuid REFERENCES teilnahme(id),
  wahl          text NOT NULL,
  CONSTRAINT stimme_wahl CHECK (wahl IN ('ja','nein','enthaltung'))
);

-- Hält fest, DASS jemand abgestimmt hat, nie WIE. Nur so lässt sich doppelte
-- Stimmabgabe auch bei geheimer Wahl verhindern.
CREATE TABLE stimmabgabe (
  abstimmung_id uuid NOT NULL REFERENCES abstimmung(id),
  teilnahme_id  uuid NOT NULL REFERENCES teilnahme(id),
  PRIMARY KEY (abstimmung_id, teilnahme_id)
);
