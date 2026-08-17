# ConferenceCue — Sitzungssystem für Gremien

Eine Sitzung wird eröffnet, die Leitung führt durch die Tagesordnung, erteilt
das Wort, das Mikrofon geht auf und die PTZ-Kamera fährt per VISCA over IP auf
den Preset des Platzes. Dazu Abstimmung und Protokoll aus der Ereigniskette.

Was noch fehlt: Ton, Bildregie, Transkript, Sitzungsmappe. Der Blick auf das
Ganze steht in `dokumentation/blockschaltbild.html`.

## Einmal ansehen

Ohne Datenbank, ohne Einrichtung, zwölf Plätze und zwei Kameras:

```sh
go run ./cmd/server -konfiguration vorfuehrung/config.yaml
```

Dann `http://localhost:8080` öffnen. Anmelden auf Platz 1 mit der PIN `0101`,
Platz 2 `0202` und so weiter. Alles liegt im Arbeitsspeicher und ist nach dem
Beenden weg — für eine echte Sitzung gehört eine Datenbank darunter.

Unter `/emulator` liegt die **Prüfstelle**: alle zwölf Geräte in einem Fenster,
fertige Abläufe zum Abspielen, dazu Ereigniskette und VISCA-Befehle beim
Entstehen. Sie öffnet je Platz eine ganz normale WebSocket-Verbindung und meldet
sich mit der PIN an — keinen Sonderweg in den Server. Was dort abgewiesen wird,
wird im Saal auch abgewiesen.

## Aufbau auf einen Blick

`dokumentation/blockschaltbild.html` im Browser öffnen: alle Bausteine, der Weg
eines Mikrofons vom Tastendruck bis zur Kamerafahrt, und was heute noch fehlt.
Eine einzelne Datei ohne Abhängigkeiten.

## Start im Betrieb

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
| `/protokoll.md` | Sitzungsprotokoll aus der Ereigniskette, als Markdown |
| `/emulator` | Prüfstelle — nur bei `emulator: true`, sie gibt die PINs preis |

Namensschild und Dolmetscherplatz lesen nur mit — sie melden keinen Platz an.

## Konfiguration

`config.yaml`: `datenbank`, `adresse`, `saal_datei`, `sitzungs_datei`,
`max_offene_mikrofone`, `kamera_zeitlimit_ms`.

Dazu zwei Schalter, die nur außerhalb des Saals etwas zu suchen haben:

| Schlüssel | Wirkung |
|---|---|
| `kamera_attrappe: true` | simulierte Kameras hören auf den Adressen aus `saal.json` und quittieren wie echte Geräte |
| `emulator: true` | schaltet `/emulator` frei. **Gibt die PINs im Klartext preis** — ohne den Schalter gibt es die Adressen nicht |

`saal.json` beschreibt Kameras und Plätze samt Presetnummer; die Presets liegen
in der Kamera, der Server merkt sich nur die Nummer. `sitzung.json` verbindet
Person, Platz, Rolle und PIN und enthält die `tagesordnung`. Die PIN liegt in
der Datenbank nur als bcrypt-Hash und geht nie an einen Client.

Eine Teilnahme kann `"aufzeichnungswiderspruch": true` tragen; ein Punkt der
Tagesordnung `"oeffentlich": false`. Fehlt die Angabe, ist der Punkt öffentlich.

## Rollen

| Rolle | darf |
|---|---|
| `leitung` (aktiv) | Sitzung führen, Tagesordnung aufrufen, Wort erteilen und entziehen, Leitung übergeben, jedes Mikrofon |
| `leitung` (nicht aktiv) | wie ein Delegierter — berechtigt sind viele, aktiv ist genau eine |
| `delegierter` | Wort melden, eigenes Mikrofon nach erteiltem Wort |
| `schriftfuehrung`, `gast` | nur zusehen, kein Stimmrecht |

## Tagesordnung

Höchstens ein Punkt ist in Arbeit; ein neuer Aufruf schließt den vorigen.
`offen → laufend → abgeschlossen | vertagt`. Ein abgeschlossener Punkt wird
wieder aufgenommen, ein vertagter nicht — der gehört in die nächste Sitzung.

Zwei Regeln hängen daran:

- **Ein nicht öffentlicher Punkt pausiert Stream und Aufzeichnung.** Das
  entscheidet der Sitzungszustand, nicht ein Handgriff an der Technik. Der
  Zustand trägt `aufzeichnung`, und die Wechsel stehen als
  `aufzeichnung_pausiert` und `aufzeichnung_fortgesetzt` in der Kette.
- **Ein Beschluss gehört an einen Punkt.** Wo es eine Tagesordnung gibt, muss
  einer aufgerufen sein, sonst weist der Kern die Abstimmung mit `kein_top` ab.
  Der Punkt steht in der Nutzlast von `abstimmung_gestartet` — nachträglich ist
  die Zuordnung nicht mehr herstellbar. Während einer laufenden Abstimmung
  wechselt der Punkt nicht.

Eine Sitzung ohne Tagesordnung läuft weiter wie bisher.

## Abstimmung

Offen, namentlich oder geheim. Beschlussfähigkeit wird beim Start geprüft und
eingefroren — wer später kommt, ändert sie nicht mehr. Quorum ist die einfache
Mehrheit der Stimmberechtigten. Solange abgestimmt wird, bleibt die Zählung
unsichtbar; erst das Auszählen zeigt das Ergebnis, das Feststellen macht es
verbindlich.

Bei **geheimer Wahl** existiert die Zuordnung Stimme zu Person nirgends: nicht
in der Tabelle `stimme`, nicht im Ereignisprotokoll, nicht im Zustand. Nur
`stimmabgabe` hält fest, *dass* jemand abgestimmt hat — sonst ließe sich
doppeltes Abstimmen nicht verhindern.

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
- **Wenn die Leitung ausfällt**, übernimmt eine andere berechtigte Person
  ausdrücklich — das System vollzieht das nie von selbst, und es steht im
  Protokoll. Solange der führende Platz besetzt ist, wird übergeben.
- **Keine Technikeingriffe während der Abstimmung.** Das Mikrofon geht auf,
  die Kamera bleibt stehen. Danach fährt sie wieder.
- **Widerspruch gegen die Aufzeichnung.** Wer widersprochen hat, wird von der
  Kameranachführung übersprungen (`kamera_uebersprungen`); das Mikrofon geht
  trotzdem auf. Der Widerspruch hängt an der Teilnahme, nicht an einer
  Einstellung, die jemand vergessen kann.
- **Kamera-Attrappe.** `kamera_attrappe: true` startet simulierte Kameras, die
  wirklich auf UDP hören, den Rahmen zerlegen und mit Acknowledge und
  Completion antworten. Der Weg bleibt derselbe wie im Saal — es fehlt nur die
  Optik. Was sie empfangen haben, steht Byte für Byte in der Prüfstelle.
- **Kein Ton.** Der Dolmetscherplatz zeigt nur, wer spricht.
