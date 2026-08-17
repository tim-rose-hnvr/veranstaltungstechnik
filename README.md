# Kameranachverfolgung — Meilenstein 1 und 2

Eine Sitzung wird eröffnet, die Leitung erteilt das Wort, das Mikrofon geht auf
und die PTZ-Kamera fährt per VISCA over IP auf den Preset des Platzes.
Kein Audio, kein Video, keine Abstimmung.

## Start

Vorausgesetzt: Go 1.24 und PostgreSQL ab 13 (wegen `gen_random_uuid()`).

```sh
createdb sitzung                # Datenbank aus config.yaml
go run ./cmd/server             # aus dem Wurzelverzeichnis starten
```

Fehlende Tabellen werden beim Start aus `migrationen/` eingespielt, danach
`saal.json` und `sitzung.json` eingelesen. Wiederholtes Starten ändert nichts.
Andere Konfiguration: `-konfiguration pfad.yaml`.

## Seiten

| Adresse | Rolle |
|---|---|
| `/` | Sprechstelle: anmelden, Wort melden, Mikrofon schalten |
| `/namensschild?platz=1` | Namensschild eines Platzes |
| `/dolmetscher` | Dolmetscherplatz, zeigt den Redner (ohne Ton) |
| `/testumgebung` | die drei Geräte nebeneinander in einem Fenster |
| `/vorabcheck` | Selbsttest vor der Sitzung: Kette, Besetzung, jede Kamera |

Namensschild und Dolmetscherplatz lesen nur mit — sie melden keinen Platz an.

## Konfiguration

`config.yaml`: `datenbank`, `adresse`, `saal_datei`, `sitzungs_datei`,
`max_offene_mikrofone`, `kamera_zeitlimit_ms`.

`saal.json` beschreibt Kameras und Plätze samt Presetnummer; die Presets liegen
in der Kamera, der Server merkt sich nur die Nummer. `sitzung.json` verbindet
Person, Platz, Rolle und PIN. Die PIN liegt in der Datenbank nur als
bcrypt-Hash und geht nie an einen Client.

## Rollen

| Rolle | darf |
|---|---|
| `leitung` (aktiv) | Sitzung führen, Wort erteilen und entziehen, Leitung übergeben, jedes Mikrofon |
| `leitung` (nicht aktiv) | wie ein Delegierter — berechtigt sind viele, aktiv ist genau eine |
| `delegierter` | Wort melden, eigenes Mikrofon nach erteiltem Wort |
| `schriftfuehrung`, `gast` | nur zusehen |

## Test

```sh
go test ./...                   # ohne Datenbank, gegen die Ablage im Speicher
createdb sitzung_test           # zusätzlich gegen echtes PostgreSQL:
SITZUNG_TEST_DB='postgres://sitzung:geheim@localhost:5432/sitzung_test?sslmode=disable' go test ./...
```

Achtung: Die Testdatenbank wird dabei geleert.

## Was zu wissen ist

- **Rechte nur im Kern.** `intern/kern` entscheidet, die Oberfläche graut nur
  aus. Das Feld `ich.darf` im Zustand kommt fertig aus dem Kern.
- **Ereigniskette**: `hash = sha256(vorgaenger_hash ‖ folge_nr ‖ zeit ‖ art ‖ nutzlast)`,
  `folge_nr` als 8 Byte big endian, Zeit als RFC3339 in UTC auf Mikrosekunden
  gestutzt, Nutzlast als kanonisches JSON. Geschrieben in einer Transaktion mit
  `SELECT … FOR UPDATE` auf den letzten Eintrag.
- **Ereignis vor Zustand.** Scheitert der Protokolleintrag, bleibt der Zustand
  unverändert und der Client bekommt `speicher_fehler`.
- **Eine Zeitachse je Sitzung.** Jedes Ereignis trägt `sitzung` und `ms` —
  Millisekunden seit der Eröffnung. Beides steht in der Nutzlast und geht
  damit in den Hash ein: die Zeitachse ist fälschungssicher, ohne dass die
  Hash-Regel sich ändert. Ohne sie ließen sich Aufzeichnung, Transkript und
  Protokoll später nicht zusammenbringen.
- **Kamera nebenher.** Eine stumme Kamera verzögert die Clients nicht, sie
  erzeugt `kamera_nicht_erreichbar` und `erreichbar: false`.
- **Eine geschlossene Sitzung wird nicht wieder eröffnet** — so will es die
  Zustandskette. Für den nächsten Durchlauf einen neuen `titel` in
  `sitzung.json` eintragen.
- **Übergabe der Leitung** braucht eine zweite Teilnahme mit der Rolle
  `leitung`; in `sitzung.json` steht bewusst nur eine.
- **Vorabcheck.** `POST /vorabcheck` fährt jeden Platz an und prüft Kette und
  Besetzung. Während einer laufenden Sitzung ist er gesperrt — er bewegt
  Kameras. Antwort 409, wenn es trotzdem jemand versucht.
- **Prüfung bei jedem Push.** `.github/workflows/pruefung.yml` baut, prüft die
  Formatierung und lässt die Tests laufen, auch gegen eine echte PostgreSQL.
- **Kein Ton.** Der Dolmetscherplatz zeigt nur, wer spricht.
