# Auftrag — Meilenstein 2

Diesen Auftrag als erste Nachricht in eine **neue Unterhaltung** geben, zusammen mit `CLAUDE.md` und `ENTSCHEIDUNGEN.md` im Projekt. Meilenstein 1 muss stehen und grün sein.

---

## Warum dieser Meilenstein vor dem Ton

Die Baureihenfolge in `CLAUDE.md` nennt als Nächstes den Ton. Das wird vorgezogen, weil zwei Dinge dagegen sprechen: Automixer, Mix-Minus und Einmessung lassen sich nur in einem Raum mit echter Anlage abnehmen, und Rollen samt Rechteprüfung gehören zu den Fundamenten, die später nicht nachrüstbar sind. Ton wird Meilenstein 3.

---

## Ziel in einem Satz

Eine Sitzung wird eröffnet, Personen sitzen auf Plätzen mit einer Rolle, die Sitzungsleitung erteilt das Wort — und erst dann lässt sich ein Mikrofon öffnen.

Kein Audio. Kein Video. Keine Abstimmung. Keine Tagesordnung.

---

## Abnahmekriterien

- [ ] `go run ./cmd/server` startet wie bisher; zusätzlich wird `sitzung.json` eingelesen und in `person`, `sitzung` und `teilnahme` geschrieben (idempotent)
- [ ] Anmeldung am Platz mit vierstelliger PIN. Falsche PIN wird abgewiesen, der Platz bleibt frei
- [ ] Der Zustand enthält Sitzung, Redeliste und die eigene Rolle; jeder Client sieht nur, was seine Rolle darf
- [ ] Wortmeldung, Worterteilung, Wortentzug und Zurückziehen funktionieren und halten die Zustandskette ein
- [ ] **Ein Mikrofon lässt sich nur öffnen, wenn der Platz das Wort hat.** Ausnahme: die aktive Sitzungsleitung darf jederzeit
- [ ] Genau eine Sitzungsleitung ist aktiv. Übergabe nur ausdrücklich, mit Protokolleintrag; die abgebende Seite verliert die Rechte im selben Moment
- [ ] Jede Rechteprüfung liegt in `intern/kern`. In `intern/web` und im Client steht keine einzige Prüfung, die über das Ausgrauen von Knöpfen hinausgeht
- [ ] Läuft keine Sitzung, ist das Schalten von Mikrofonen gesperrt
- [ ] Die Grenze aus Meilenstein 1 gilt weiter: nie mehr als `max_offene_mikrofone`
- [ ] Jede Zustandsänderung erzeugt weiterhin eine Zeile in `ereignis` mit korrektem `vorgaenger_hash`
- [ ] `go test ./...` läuft grün, inklusive der unten genannten Tests
- [ ] `README.md` bleibt bei höchstens einer Bildschirmseite

---

## Nicht im Umfang

Nicht bauen, auch nicht vorbereiten, auch nicht als Platzhalter:

Audio jeder Art · Video jeder Art · Abstimmung · Tagesordnung · Redezeit · Sitzungsmappe · Transkript · Aufzeichnung · Hauptversammlung · Vertretung und Stimmübertragung · NFC · Wallet · Zertifikate · native App · Mehrere Säle gleichzeitig

Der Client bleibt **HTML mit reinem JavaScript**. Kein Framework, kein Build-Schritt.

---

## Verzeichnisstruktur

Neu gegenüber Meilenstein 1:

```
/migrationen/002_sitzung.sql
/sitzung.json
/intern/kern/rolle.go        Rollen und Rechte
/intern/kern/sitzung.go      Sitzungszustand, Redeliste
```

---

## Konfiguration

`config.yaml` bekommt genau eine Zeile dazu:

```yaml
sitzungs_datei: sitzung.json
```

`sitzung.json`
```json
{
  "titel": "Vorstandssitzung",
  "teilnahmen": [
    { "platz": 1, "person": "Anke Bergmann", "rolle": "leitung",     "pin": "1234" },
    { "platz": 2, "person": "Jonas Öztürk",  "rolle": "delegierter", "pin": "2345" },
    { "platz": 3, "person": "Rita Falk",     "rolle": "schriftfuehrung", "pin": "3456" }
  ]
}
```

Die PIN steht im Klartext in der Datei — das ist für diesen Meilenstein zulässig, weil noch keine echte Anmeldung existiert. In der Datenbank wird sie als Hash abgelegt, nie im Klartext, und sie geht **nie** an einen Client.

---

## Datenbank

`migrationen/002_sitzung.sql` — genau diese Tabellen, nicht mehr:

```sql
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
```

`folge_nr` bestimmt die Reihenfolge in der Redeliste. Sie wird wie bei `ereignis` in einer Transaktion vergeben.

**Die aktive Sitzungsleitung** bekommt keine eigene Spalte. Sie ergibt sich aus dem Ereignisprotokoll: die zuletzt erteilte Leitung gilt. Beim Start wird sie aus der Kette gelesen; gibt es keinen Eintrag, ist es die Teilnahme mit der Rolle `leitung`.

---

## Rollen und Rechte

Genau diese Rechte, im Kern geprüft:

| Recht | leitung (aktiv) | delegierter | schriftfuehrung | gast |
|---|---|---|---|---|
| Sitzung eröffnen, unterbrechen, schließen | ja | nein | nein | nein |
| Wort erteilen, entziehen | ja | nein | nein | nein |
| Leitung übergeben | ja | nein | nein | nein |
| Wort melden, zurückziehen | ja | ja | nein | nein |
| Mikrofon öffnen | jederzeit | nur mit erteiltem Wort | nein | nein |
| Zustand sehen | ja | ja | ja | ja |

Eine Person mit der Rolle `leitung`, die den Staffelstab **nicht** hat, ist berechtigt, aber nicht aktiv: Sie darf nichts von dem, was in der Spalte „leitung (aktiv)" steht.

---

## WebSocket-Protokoll

Bestehende Nachrichten bleiben. Geändert und neu:

**Client zum Server**

```json
{ "typ": "anmelden",           "platz": 2, "pin": "2345" }
{ "typ": "wort_melden" }
{ "typ": "wort_zurueckziehen" }
{ "typ": "wort_erteilen",      "platz": 2 }
{ "typ": "wort_entziehen",     "platz": 2 }
{ "typ": "leitung_uebergeben", "platz": 3 }
{ "typ": "sitzung_eroeffnen" }
{ "typ": "sitzung_schliessen" }
```

`wort_melden` und `wort_zurueckziehen` brauchen keinen Platz — sie gelten für den angemeldeten Platz der Verbindung.

**Server zum Client** — nach jeder Änderung an alle, weiterhin vollständig:

```json
{
  "typ": "zustand",
  "stand": 47,
  "max_offen": 3,
  "sitzung": { "titel": "Vorstandssitzung", "zustand": "laufend", "leitung_platz": 1 },
  "ich": { "platz": 2, "rolle": "delegierter", "darf": ["wort_melden", "mikro_an"] },
  "plaetze": [
    { "nummer": 1, "name": "Vorsitz", "person": "Anke Bergmann", "mikro": false, "belegt": true, "hat_wort": false }
  ],
  "redeliste": [
    { "platz": 2, "person": "Jonas Öztürk", "zustand": "erteilt" }
  ],
  "kamera": { "name": "PTZ Mitte", "preset": 2, "erreichbar": true }
}
```

Das Feld `ich` ist je Verbindung verschieden — das ist die einzige Stelle, an der nicht alle dasselbe bekommen. `darf` wird im Kern berechnet, damit der Client Knöpfe ausgrauen kann, ohne selbst zu entscheiden.

Neue Fehlercodes zusätzlich zu den bestehenden: `nicht_berechtigt`, `pin_falsch`, `kein_wort`, `sitzung_laeuft_nicht`.

---

## Tests

Diese fünf müssen existieren, zusätzlich zu denen aus Meilenstein 1:

1. `TestNurAktiveLeitungErteiltDasWort` — ein Delegierter und eine berechtigte, aber nicht aktive Leitung werden mit `nicht_berechtigt` abgewiesen; der Zustand bleibt unverändert
2. `TestStaffelstabGenauEineLeitung` — nach der Übergabe darf die neue Leitung alles, die alte nichts mehr; beides in einem Ereignis festgehalten
3. `TestMikroNurNachWorterteilung` — ohne erteiltes Wort wird `mikro_an` mit `kein_wort` abgewiesen; die aktive Leitung darf trotzdem
4. `TestWortmeldungZustandskette` — jeder erlaubte Übergang funktioniert, jeder unerlaubte wird abgewiesen (etwa `beendet → laufend`)
5. `TestSitzungImportIdempotent` — zweimaliger Import derselben `sitzung.json` erzeugt keine zusätzlichen Zeilen, und die PIN steht nirgends im Klartext

Weiterhin gilt: keine echte Datenbank und keine echte Kamera in den Tests, beides über die vorhandenen Schnittstellen.

---

## Arbeitsregeln für diese Aufgabe

- **Fragen bündeln.** Erst den ganzen Auftrag lesen, dann höchstens drei blockierende Fragen auf einmal. Was in `ENTSCHEIDUNGEN.md` steht, ist beantwortet.
- **Keine Erweiterungen.** Was oben nicht steht, wird nicht gebaut.
- **Ganze Dateien ausgeben**, nicht Ausschnitte.
- **Reihenfolge:** Migration → Sitzungsimport → Rollen und Rechte im Kern → Wortmeldung und Redeliste → Protokoll erweitern → Client zuletzt. Nach jedem Schritt lauffähig.
- **Meilenstein 1 bleibt grün.** Keine bestehende Abnahme darf brechen; die vier Tests von dort laufen weiter.
- **Wenn etwas an diesem Auftrag technisch nicht haltbar ist:** sagen, nicht stillschweigend anders bauen.
