-- Der Saalplan braucht Geometrie. Bisher hatte ein Platz nur eine Nummer, und
-- aus einer Nummer lässt sich keine Sitzordnung ableiten — jeder Versuch wäre
-- geraten.
--
-- reihe und spalte beschreiben einen rechteckigen Tisch: zwei Längsseiten
-- (oben, unten) und zwei Kopfenden (links, rechts). Das deckt Vorstandszimmer
-- und Ratssaal ab. Für Hufeisen oder Amphitheater reicht es nicht; das wird
-- eine eigene Form, wenn ein Kunde sie hat.
--
-- Fehlt die Angabe, zeigt die Oberfläche eine einfache Kachelreihe statt eines
-- Plans. Ein falscher Plan wäre schlimmer als keiner.
ALTER TABLE platz
  ADD COLUMN IF NOT EXISTS reihe text,
  ADD COLUMN IF NOT EXISTS spalte int;

ALTER TABLE platz
  ADD CONSTRAINT platz_reihe CHECK (reihe IS NULL OR reihe IN ('oben','unten','links','rechts'));

CREATE TABLE saalplan_geprueft (bemerkung text);
