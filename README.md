# Kameranachverfolgung — Meilenstein 1

Ein Browser meldet „Mikrofon an", der Server erkennt den Sitzplatz und fährt
die PTZ-Kamera per VISCA over IP auf den zugehörigen Preset.
Kein Audio, kein Video, keine Abstimmung.

## Start

Vorausgesetzt: Go 1.24 und PostgreSQL ab 13 (wegen `gen_random_uuid()`).

```sh
createdb sitzung                # Datenbank aus config.yaml
go run ./cmd/server             # aus dem Wurzelverzeichnis starten
```

Beim ersten Start wird `migrationen/001_grundlage.sql` eingespielt, falls die
Tabellen fehlen, und `saal.json` eingelesen. Wiederholtes Starten ändert
nichts. Andere Konfiguration: `-konfiguration pfad.yaml`.

## Seiten

| Adresse | Rolle |
|---|---|
| `/` | Sprechstelle: Platz wählen, Mikrofon schalten |
| `/namensschild?platz=1` | Namensschild eines Platzes, zeigt „spricht" |
| `/dolmetscher` | Dolmetscherplatz, zeigt den Redner (ohne Ton) |
| `/testumgebung` | die drei Geräte nebeneinander in einem Fenster |

Namensschild und Dolmetscherplatz lesen nur mit — sie melden keinen Platz an
und belegen deshalb keinen. Jede Seite läuft genauso auf echter Hardware.

## Konfiguration

`config.yaml`: `datenbank`, `adresse`, `saal_datei`, `max_offene_mikrofone`,
`kamera_zeitlimit_ms`.

`saal.json` beschreibt Saal, Kameras und Plätze samt Presetnummer. Die Presets
liegen in der Kamera, der Server merkt sich nur die Nummer.

## Test

```sh
go test ./...                   # ohne Datenbank, gegen die Ablage im Speicher
createdb sitzung_test           # zusätzlich gegen echtes PostgreSQL:
SITZUNG_TEST_DB='postgres://sitzung:geheim@localhost:5432/sitzung_test?sslmode=disable' go test ./...
```

Achtung: Die Testdatenbank wird dabei geleert.

## Was zu wissen ist

- **Ereigniskette**: `hash = sha256(vorgaenger_hash ‖ folge_nr ‖ zeit ‖ art ‖ nutzlast)`;
  `folge_nr` als 8 Byte big endian, Zeit als RFC3339 mit Nanosekunden in UTC
  auf Mikrosekunden gestutzt, Nutzlast als kanonisches JSON. Geschrieben in
  einer Transaktion mit `SELECT … FOR UPDATE` auf den letzten Eintrag.
- **Ereignis vor Zustand**: Scheitert der Protokolleintrag, bleibt der Zustand
  unverändert und der Client bekommt `speicher_fehler`.
- **Kamera nebenher**: Der Kamerabefehl läuft in einer eigenen Routine. Eine
  stumme Kamera verzögert die Clients nicht, sie erzeugt das Ereignis
  `kamera_nicht_erreichbar` und `erreichbar: false`.
- **Kein Ton**: Der Dolmetscherplatz zeigt nur, wer spricht. Kanalwahl und
  Hustentaste sind sichtbar abgeschaltet, bis es den Audioteil gibt.
- **Der Saal hängt von nichts ab**: keine externe Schriftart, kein CDN, kein
  Browser-Speicher in der Oberfläche.
