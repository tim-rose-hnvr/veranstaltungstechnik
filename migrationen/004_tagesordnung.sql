-- Wer der Aufzeichnung widerspricht, wird von der Kameranachführung
-- übersprungen. Das ist keine Einstellung der Oberfläche, sondern eine
-- Eigenschaft der Teilnahme — sie gilt für jedes Gerät und jeden Neustart.
ALTER TABLE teilnahme
  ADD COLUMN IF NOT EXISTS aufzeichnungswiderspruch boolean NOT NULL DEFAULT false;

-- Die Tagesordnung ordnet die Sitzung. Ein Beschluss ohne Punkt ist später
-- nicht mehr zuzuordnen, deshalb hängt jede Abstimmung an einem.
--
-- oeffentlich steuert Stream und Aufzeichnung: bei einem nicht öffentlichen
-- Punkt pausieren beide automatisch aus dem Sitzungszustand heraus.
CREATE TABLE tagesordnungspunkt (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sitzung_id  uuid NOT NULL REFERENCES sitzung(id),
  nummer      int NOT NULL,
  titel       text NOT NULL,
  oeffentlich boolean NOT NULL DEFAULT true,
  zustand     text NOT NULL DEFAULT 'offen',
  beginn      timestamptz,
  ende        timestamptz,
  UNIQUE (sitzung_id, nummer),
  CONSTRAINT top_zustand CHECK (zustand IN ('offen','laufend','abgeschlossen','vertagt'))
);
