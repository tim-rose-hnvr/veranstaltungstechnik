-- Die geheime Wahl war aufdeckbar. Nicht durch einen Programmierfehler in
-- einer Abfrage, sondern durch eine Eigenschaft von PostgreSQL selbst.
--
-- Jede Tabelle führt die Systemspalte xmin: die Nummer der Transaktion, die
-- eine Zeile geschrieben hat. Zwei Zeilen aus derselben Transaktion tragen
-- denselben Wert. Bisher schrieb eine Stimmabgabe in einer Transaktion
--
--   INSERT INTO stimmabgabe (abstimmung_id, teilnahme_id)   -- WER
--   INSERT INTO stimme      (abstimmung_id, wahl)           -- WAS
--
-- und beide Zeilen trugen damit dasselbe xmin. Eine einzige Abfrage genügte:
--
--   SELECT a.teilnahme_id, s.wahl FROM stimmabgabe a
--   JOIN stimme s ON a.xmin = s.xmin AND a.abstimmung_id = s.abstimmung_id
--
-- Das deckte jede Stimme auf. Wer die Datenbank lesen darf — die
-- Systemverwaltung des Kunden, ein Sicherungsband, eine Beschlagnahme —
-- konnte die geheime Wahl vollständig rekonstruieren. Genau das darf es
-- laut Leitsatz nirgends geben.
--
-- Die Abhilfe: bei geheimer Wahl entsteht überhaupt keine Zeile mehr, die
-- eine einzelne Stimme festhält. Gezählt wird in drei Zählern, und jede
-- Stimmabgabe rührt ALLE DREI an — auch die beiden, die nicht gewählt
-- wurden, mit einem Zuschlag von null. Damit tragen alle drei Zeilen nach
-- jeder Stimmabgabe dasselbe xmin, und aus xmin folgt nur noch, DASS
-- jemand abgestimmt hat. Das stand ohnehin schon in stimmabgabe.
--
-- Was damit NICHT verteidigt ist, offen gesagt: wer als Superuser mit
-- pageinspect die toten Zeilenversionen vor dem naechsten VACUUM ausliest,
-- sieht die Zwischenstaende und kann daraus zurueckrechnen. Dagegen hilft
-- kein Tabellenentwurf, sondern nur der Zugriffsschutz der Datenbank selbst.
-- Der Weg ueber ein blind signiertes Einmal-Token, den die Leitlinien
-- vorsehen, schliesst auch diese Luecke und bleibt der naechste Schritt.
CREATE TABLE stimme_zaehler (
  abstimmung_id uuid NOT NULL REFERENCES abstimmung(id),
  wahl          text NOT NULL,
  anzahl        int NOT NULL DEFAULT 0,
  PRIMARY KEY (abstimmung_id, wahl),
  CONSTRAINT stimme_zaehler_wahl CHECK (wahl IN ('ja','nein','enthaltung'))
);

-- Altbestand: geheime Stimmen, die noch einzeln gespeichert sind, werden in
-- Zähler überführt und die Einzelzeilen gelöscht. Danach ist die Zuordnung
-- auch für zurückliegende Wahlen nicht mehr herstellbar.
INSERT INTO stimme_zaehler (abstimmung_id, wahl, anzahl)
SELECT s.abstimmung_id, s.wahl, count(*)
FROM stimme s JOIN abstimmung a ON a.id = s.abstimmung_id
WHERE a.art = 'geheim'
GROUP BY s.abstimmung_id, s.wahl
ON CONFLICT (abstimmung_id, wahl) DO NOTHING;

DELETE FROM stimme s
USING abstimmung a
WHERE a.id = s.abstimmung_id AND a.art = 'geheim';
