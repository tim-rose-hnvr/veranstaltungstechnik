# Kameranachverfolgung — Meilenstein 1

Ein Browser meldet „Mikrofon an", der Server erkennt den Sitzplatz und fährt
die PTZ-Kamera per VISCA over IP auf den zugehörigen Preset.
Kein Audio, kein Video, keine Abstimmung.

## Start

Vorausgesetzt: Go 1.24 und eine laufende PostgreSQL (ab Version 13, wegen
`gen_random_uuid()`).

```sh
createdb sitzung                # Datenbank aus config.yaml
go run ./cmd/server             # aus dem Wurzelverzeichnis starten
```

Beim ersten Start spielt der Server `migrationen/001_grundlage.sql` ein, wenn
die Tabellen fehlen, und liest `saal.json` ein. Wiederholtes Starten ändert
nichts. Danach `http://localhost:8080` öffnen — mehrere Browser gleichzeitig
sehen denselben Zustand.

## Konfiguration

`config.yaml`

| Feld | Bedeutung |
|---|---|
| `datenbank` | Verbindung zu PostgreSQL |
| `adresse` | Adresse des Webservers |
| `saal_datei` | Pfad zu `saal.json` |
| `max_offene_mikrofone` | Höchstzahl gleichzeitig offener Mikrofone |
| `kamera_zeitlimit_ms` | Wartezeit auf die Antwort der Kamera |

`saal.json` beschreibt Saal, Kameras und Plätze samt Presetnummer. Die
Presets liegen in der Kamera, der Server merkt sich nur die Nummer.
Ein anderer Pfad zur Konfiguration: `go run ./cmd/server -konfiguration pfad.yaml`.

## Test

```sh
go test ./...
```

Läuft ohne Datenbank gegen eine Ablage im Arbeitsspeicher, die dieselben
Eindeutigkeiten durchsetzt wie das SQL-Schema. Zusätzlich gegen echtes
PostgreSQL:

```sh
createdb sitzung_test
SITZUNG_TEST_DB='postgres://sitzung:geheim@localhost:5432/sitzung_test?sslmode=disable' go test ./...
```

Achtung: Diese Datenbank wird beim Test geleert.

## Was zu wissen ist

- **Ereigniskette**: `hash = sha256(vorgaenger_hash ‖ folge_nr ‖ zeit ‖ art ‖ nutzlast)`;
  `folge_nr` als 8 Byte big endian, Zeit als RFC3339 mit Nanosekunden in UTC
  auf Mikrosekunden gestutzt, Nutzlast als kanonisches JSON. Geschrieben wird
  in einer Transaktion mit `SELECT … FOR UPDATE` auf den letzten Eintrag.
- **Ereignis vor Zustand**: Scheitert der Protokolleintrag, bleibt der Zustand
  unverändert und der Client bekommt `speicher_fehler`. Einen Zustandswechsel
  ohne Protokolleintrag gibt es nicht.
- **Kamera nebenher**: Der Kamerabefehl läuft in einer eigenen Routine. Eine
  stumme Kamera verzögert die Antwort an die Clients nicht, sie erzeugt das
  Ereignis `kamera_nicht_erreichbar` und `erreichbar: false`.
- **Der Saal hängt von nichts ab**: keine externe Schriftart, kein CDN, kein
  Browser-Speicher in der Oberfläche.
