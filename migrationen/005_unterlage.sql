-- Die Sitzungsmappe. Der Inhalt liegt auf der Platte, hier steht, wo er liegt,
-- wie vertraulich er ist und zu welchem Punkt er gehört.
--
-- pruefsumme ist der SHA-256 der Datei beim Einlesen. Weicht sie beim Abruf ab,
-- wurde die Datei unter dem laufenden System ausgetauscht — dann wird nicht
-- ausgeliefert.
--
-- Ein Zugriffsprotokoll gibt es hier bewusst nicht: wer wann welche Unterlage
-- geöffnet hat, steht in der Ereigniskette. Eine zweite Liste daneben könnte
-- auseinanderlaufen, und nur die Kette ist fälschungssicher.
CREATE TABLE unterlage (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sitzung_id uuid NOT NULL REFERENCES sitzung(id),
  top_id     uuid REFERENCES tagesordnungspunkt(id),
  nummer     int NOT NULL,
  titel      text NOT NULL,
  datei      text NOT NULL,
  dateiname  text NOT NULL,
  typ        text NOT NULL DEFAULT 'application/octet-stream',
  groesse    bigint NOT NULL DEFAULT 0,
  stufe      text NOT NULL DEFAULT 'intern',
  pruefsumme text NOT NULL,
  UNIQUE (sitzung_id, datei),
  CONSTRAINT unterlage_stufe CHECK (stufe IN ('oeffentlich','intern','vertraulich','geheim'))
);

CREATE INDEX unterlage_nach_top ON unterlage (sitzung_id, top_id, nummer);
