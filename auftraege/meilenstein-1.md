# Auftrag — Meilenstein 1

Diesen Auftrag als erste Nachricht in eine **neue Unterhaltung** geben, zusammen mit `CLAUDE.md` und `ENTSCHEIDUNGEN.md` im Projekt.

---

## Ziel in einem Satz

Ein Browser-Client meldet „Mikrofon an", der Server erkennt den Sitzplatz und fährt eine PTZ-Kamera per VISCA over IP auf den zugehörigen Preset.

Kein Audio. Kein Video. Keine Abstimmung. Keine Datenbank-Migrationen über das Nötige hinaus.

---

## Abnahmekriterien

Fertig ist der Meilenstein, wenn alle Punkte erfüllt sind:

- [ ] `go run ./cmd/server` startet ohne Fremdkonfiguration außer `config.yaml` und einer laufenden PostgreSQL
- [ ] Beim Start wird `saal.json` eingelesen und in die Tabellen `saal`, `platz`, `kamera`, `preset` geschrieben (idempotent, wiederholtes Starten ändert nichts)
- [ ] `GET /` liefert eine Seite, auf der ein Sitzplatz gewählt und ein Mikrofon geschaltet werden kann
- [ ] Mehrere gleichzeitig geöffnete Browser sehen denselben Zustand in unter 200 ms
- [ ] Beim Öffnen eines Mikrofons wird der Preset der zuständigen Kamera abgerufen
- [ ] Mehr als `max_offene_mikrofone` gleichzeitig ist nicht möglich; der Versuch liefert einen Fehler, keinen Absturz
- [ ] Ist keine Kamera erreichbar, läuft alles Übrige weiter und es wird protokolliert
- [ ] Jede Zustandsänderung erzeugt eine Zeile in `ereignis` mit korrektem `vorgaenger_hash`
- [ ] `go test ./...` läuft grün, inklusive der unten genannten Tests
- [ ] `README.md` beschreibt Start, Konfiguration und Test in höchstens einer Bildschirmseite

---

## Nicht im Umfang

Nicht bauen, auch nicht vorbereiten, auch nicht als Platzhalter:

Audio jeder Art · Video jeder Art · WebRTC · Abstimmung · Sitzungsmappe · Transkript · Teams · Authentifizierung über Zertifikate · Mandantenfähigkeit über die Spalte hinaus · native App · Docker-Compose-Landschaften · Kubernetes · Frontend-Framework

Der Client ist **eine HTML-Datei mit reinem JavaScript**. Kein React, kein Build-Schritt.

---

## Verzeichnisstruktur

```
/cmd/server/main.go
/intern/kern/          Zustand, Regeln, Ereignisse
/intern/kamera/        VISCA over IP
/intern/speicher/      PostgreSQL
/intern/web/           HTTP + WebSocket
/web/index.html        Client, eine Datei
/migrationen/001_grundlage.sql
/config.yaml
/saal.json
```

---

## Konfiguration

`config.yaml`
```yaml
datenbank: postgres://sitzung:geheim@localhost:5432/sitzung?sslmode=disable
adresse: ":8080"
saal_datei: saal.json
max_offene_mikrofone: 3
kamera_zeitlimit_ms: 500
```

`saal.json`
```json
{
  "saal": "Testraum",
  "kameras": [
    { "name": "PTZ Mitte", "adresse": "192.168.1.50:52381", "kanal": 1 }
  ],
  "plaetze": [
    { "nummer": 1, "name": "Vorsitz",  "kamera": "PTZ Mitte", "preset": 1 },
    { "nummer": 2, "name": "Platz 2",  "kamera": "PTZ Mitte", "preset": 2 },
    { "nummer": 3, "name": "Platz 3",  "kamera": "PTZ Mitte", "preset": 3 }
  ]
}
```

---

## Datenbank

`migrationen/001_grundlage.sql` — genau diese Tabellen, nicht mehr:

```sql
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
```

**Hash-Regel:** `hash = sha256(vorgaenger_hash || folge_nr || zeit_rfc3339nano || art || nutzlast_kanonisches_json)`. Beim ersten Ereignis ist `vorgaenger_hash` NULL. Schreiben nur in einer Transaktion mit `SELECT ... FOR UPDATE` auf den letzten Eintrag, damit die Kette lückenlos bleibt.

---

## WebSocket-Protokoll

Endpunkt `GET /ws`. Ein JSON-Objekt je Nachricht, Feld `typ` entscheidet.

**Client zum Server**

```json
{ "typ": "anmelden",  "platz": 2 }
{ "typ": "mikro_an",  "platz": 2 }
{ "typ": "mikro_aus", "platz": 2 }
```

**Server zum Client** — nach jeder Änderung an alle:

```json
{
  "typ": "zustand",
  "stand": 47,
  "max_offen": 3,
  "plaetze": [
    { "nummer": 1, "name": "Vorsitz", "mikro": false, "belegt": false },
    { "nummer": 2, "name": "Platz 2", "mikro": true,  "belegt": true  }
  ],
  "kamera": { "name": "PTZ Mitte", "preset": 2, "erreichbar": true }
}
```

Fehler:
```json
{ "typ": "fehler", "code": "grenze_erreicht", "text": "3 von 3 Mikrofonen offen" }
```

Codes: `grenze_erreicht`, `platz_unbekannt`, `platz_belegt`.

Der Server sendet **immer den vollständigen Zustand**, keine Teiländerungen. `stand` zählt monoton hoch; der Client verwirft ältere Nachrichten.

---

## VISCA over IP

UDP an die konfigurierte Adresse, Standardport 52381.

Nutzlast für Preset abrufen, Kanal `k`, Preset `p`:
```
0x80|k  0x01  0x04  0x3F  0x02  p  0xFF
```
Für Kanal 1 also `81 01 04 3F 02 pp FF`.

Davor der 8-Byte-Kopf:
```
0x01 0x00              Nutzlasttyp „VISCA command"
0xLL 0xLL              Länge der Nutzlast, big endian
0xSS 0xSS 0xSS 0xSS    Folgenummer, big endian, je Verbindung hochzählend
```

Antwort abwarten mit `kamera_zeitlimit_ms`. Kommt nichts, ist das **kein Fehler des Systems**: Ereignis `kamera_nicht_erreichbar` schreiben, Feld `erreichbar` auf false, weitermachen.

---

## Tests

Diese vier müssen existieren:

1. `TestGrenzeOffenerMikrofone` — das vierte Mikrofon bei `max_offene_mikrofone: 3` wird abgelehnt, Zustand bleibt unverändert
2. `TestEreigniskette` — hundert Ereignisse hintereinander, Kette prüfbar, keine Lücke in `folge_nr`
3. `TestViscaRahmen` — der erzeugte Rahmen für Kanal 1, Preset 5 entspricht Byte für Byte der Vorgabe oben
4. `TestSaalImportIdempotent` — zweimaliger Import derselben `saal.json` erzeugt keine zusätzlichen Zeilen

Die Kamera in Tests über ein Interface abbilden, kein echtes UDP.

---

## Arbeitsregeln für diese Aufgabe

- **Fragen bündeln.** Erst den ganzen Auftrag lesen, dann höchstens drei blockierende Fragen auf einmal stellen. Was in `ENTSCHEIDUNGEN.md` steht, ist beantwortet und wird nicht erneut gefragt.
- **Keine Erweiterungen.** Was oben nicht steht, wird nicht gebaut — auch nicht „schon mal vorbereitet".
- **Ganze Dateien ausgeben**, nicht Ausschnitte, damit sie direkt übernommen werden können.
- **Reihenfolge:** Migration und Schema → Saal-Import → Zustand im Speicher → WebSocket → Client → VISCA zuletzt. Nach jedem Schritt lauffähig.
- **Wenn etwas an diesem Auftrag technisch nicht haltbar ist:** sagen, nicht stillschweigend anders bauen.
