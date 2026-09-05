# Datenmodell

Der Stand, wie er in `migrationen/` wirklich steht. Was hier fehlt, ist nicht
gebaut — diese Datei beschreibt nichts, was es nicht gibt.

Die maßgebliche Quelle bleibt `migrationen/*.sql`; die Liste der Migrationen
steht in `intern/speicher/postgres.go` und gilt für Server und Test
gleichermaßen. Änderungen gehören in eine neue Migration, nie in eine alte.

---

## Der tragende Satz

**Person ≠ Platz ≠ Gerät.** Der Sitzplatz trägt die Identität: Nummer,
Funktion, Stimmrecht, Kamerapreset. Personen werden je Sitzung zugeordnet,
Geräte sind austauschbar. Diese Trennung ist der Grund für die Tabelle
`teilnahme` — sie verbindet Person, Platz und Rolle **für genau eine Sitzung**.

```
organisation ─┬─ saal ─┬─ platz ─── preset ─── kamera
              │        └─ ereignis  (Hash-Kette je Saal)
              └─ person
                        sitzung ─┬─ teilnahme (person + platz + rolle)
                                 ├─ wortmeldung
                                 ├─ tagesordnungspunkt ─── unterlage
                                 └─ abstimmung ─┬─ stimme
                                                └─ stimmabgabe
```

`organisation_id` steht in `saal` und `person` — Mandantenfähigkeit ist ein
nicht nachrüstbares Fundament, deshalb von der ersten Migration an.

---

## Saal und Raum (`001_grundlage.sql`, `006_saalplan.sql`)

| Tabelle | Zweck | Bemerkenswert |
|---|---|---|
| `organisation` | Mandant | Wurzel jeder Zuordnung |
| `saal` | Raum | eindeutig je Organisation |
| `kamera` | PTZ-Kamera | `adresse` samt Port, `kanal` für VISCA |
| `platz` | Sitzplatz | `nummer` eindeutig je Saal; dazu `reihe` und `spalte` |
| `preset` | Kameraposition für einen Platz | liegt **auch** in der Kamera selbst |

`platz.reihe` ist auf `oben`, `unten`, `links`, `rechts` beschränkt,
`platz.spalte` ist die Position darin. Fehlt beides, zeigt die Oberfläche
eine Kachelreihe statt eines Saalplans — ein geratener Plan wäre schlimmer
als keiner.

Das Preset liegt in der Kamera, nicht nur im Server: fällt der Server aus,
bleiben die gelernten Positionen erhalten.

---

## Ereigniskette (`001_grundlage.sql`)

```sql
ereignis (id, saal_id, folge_nr, zeit, art, nutzlast jsonb,
          vorgaenger_hash bytea, hash bytea, UNIQUE (saal_id, folge_nr))
```

Nur anfügbar. Kein `UPDATE`, kein `DELETE` — jeder Zustandswechsel des Systems
schreibt zuerst hier, dann erst ändert sich der Zustand. Scheitert das
Schreiben, bleibt der Zustand, wie er war.

Der Hash ist

```
sha256( vorgaenger_hash ‖ folge_nr (8 Byte big endian)
        ‖ zeit (RFC3339 mit Nanosekunden, UTC)
        ‖ art ‖ nutzlast (kanonisches JSON) )
```

Zwei Feinheiten, die leicht zu übersehen und teuer zu reparieren sind:
`ZeitStutzen` kürzt auf Mikrosekunden, weil PostgreSQL `timestamptz` nicht
feiner speichert — ohne den Schnitt wäre der gehashte Zeitwert ein anderer als
der gelesene und die Kette nach dem Neuladen nicht mehr prüfbar. Und
`encoding/json` sortiert Map-Schlüssel und setzt keine Leerzeichen, womit die
kanonische Form auch nach dem Umweg über `jsonb` reproduzierbar bleibt.

**Die eine Zeitachse je Sitzung** steckt in der Nutzlast, nicht in einer
eigenen Spalte: jedes Ereignis trägt `sitzung`, und sobald die Sitzung eröffnet
ist, auch `ms` — Millisekunden seit dem Nullpunkt. Vor der Eröffnung gibt es
keinen Nullpunkt und daher kein `ms`. Weil die Nutzlast in den Hash eingeht,
ist die Achse ohne Änderung der Hash-Regel fälschungssicher. Nachträglich wäre
sie nicht mehr herstellbar, deshalb steht sie von Anfang an drin.

Der Abschluss der Kette (`intern/siegel`) steht selbst als Ereignis der Art
`kette_gesiegelt` **in** der Kette — eine zweite Liste daneben könnte
auseinanderlaufen.

---

## Sitzung und Teilnahme (`002_sitzung.sql`, `004_tagesordnung.sql`)

```sql
sitzung   (id, saal_id, titel, zustand, beginn, ende)
teilnahme (id, sitzung_id, person_id, platz_id, rolle, zustand, pin_hash,
           aufzeichnungswiderspruch,
           UNIQUE (sitzung_id, person_id), UNIQUE (sitzung_id, platz_id))
```

`sitzung.beginn` ist der Nullpunkt der Zeitachse. Solange die Sitzung nicht
eröffnet ist, steht er auf NULL.

Die beiden `UNIQUE`-Bedingungen sagen die Fachregel: eine Person sitzt in einer
Sitzung an höchstens einem Platz, und ein Platz trägt höchstens eine Person.

`aufzeichnungswiderspruch` hängt an der **Teilnahme**, nicht an einer
Einstellung der Oberfläche — er gilt damit für jedes Gerät und jeden Neustart.
Wer widersprochen hat, wird von der Kameranachführung übersprungen; das
Mikrofon geht trotzdem auf, denn Reden und Gefilmtwerden sind zweierlei.

### Zustandsketten

| Objekt | Kette |
|---|---|
| Sitzung | `vorbereitet → bereit → laufend ⇄ unterbrochen → geschlossen → archiviert` |
| Teilnahme | `eingeladen → registriert → angemeldet → anwesend → abwesend` |
| Tagesordnungspunkt | `offen → laufend → abgeschlossen \| vertagt` |
| Wortmeldung | `gemeldet → erteilt → laufend → beendet \| entzogen \| zurueckgezogen` |
| Abstimmung | `vorbereitet → laufend → ausgezaehlt → festgestellt \| abgebrochen` |

Ein abgeschlossener Tagesordnungspunkt wird wieder aufrufbar — Wiederaufnahme
kommt vor. Ein vertagter nicht: er gehört in die nächste Sitzung.

### Redeliste

```sql
wortmeldung (id, sitzung_id, teilnahme_id, folge_nr, zustand,
             gemeldet, erteilt, beendet, UNIQUE (sitzung_id, folge_nr))
```

`folge_nr` ist die Reihenfolge der Meldungen und damit die Redeliste selbst.
Beim Neustart lädt der Server die Zustände `gemeldet`, `erteilt` und `laufend`
zurück — die abgeschlossenen bleiben liegen, sie sind Geschichte.

### Tagesordnung

```sql
tagesordnungspunkt (id, sitzung_id, nummer, titel, oeffentlich, zustand,
                    beginn, ende, UNIQUE (sitzung_id, nummer))
```

`oeffentlich` steuert Stream und Aufzeichnung: bei einem nicht öffentlichen
Punkt pausieren beide **automatisch aus dem Sitzungszustand heraus** — nicht
durch einen Handgriff, den jemand vergessen kann.

---

## Abstimmung (`003_abstimmung.sql`)

```sql
abstimmung  (id, sitzung_id, folge_nr, titel, art, zustand,
             stimmberechtigt, quorum, anwesend, beginn, ende)
stimme      (id, abstimmung_id, teilnahme_id NULL, wahl)
stimmabgabe (abstimmung_id, teilnahme_id, PRIMARY KEY beide)
```

`stimmberechtigt`, `quorum` und `anwesend` werden **beim Start eingefroren**.
Wer später kommt, ändert die Beschlussfähigkeit einer laufenden Abstimmung
nicht mehr.

Die Trennung in zwei Tabellen ist der Kern der geheimen Wahl:

- `stimme` sagt **was** gewählt wurde. Bei geheimer Wahl bleibt
  `teilnahme_id` leer.
- `stimmabgabe` sagt **dass** ein Platz abgestimmt hat — nie, wie. Ohne diese
  Liste ließe sich doppelte Stimmabgabe nicht verhindern.

Beides zusammenzuführen ist bei geheimer Wahl nicht schwer gemacht, sondern
unmöglich: die Verbindung existiert nirgends, auch nicht im Ereignisprotokoll
und nicht im Zustand zum Client.

Die **Marke** des Offline-Puffers (siehe unten) steht bewusst in keiner
Tabelle und in keinem Ereignis — sie lebt nur im Arbeitsspeicher des Servers,
weil sie sonst bei geheimer Wahl ein Bindeglied zur Person wäre.

---

## Sitzungsmappe (`005_unterlage.sql`)

```sql
unterlage (id, sitzung_id, top_id, nummer, titel, datei, dateiname,
           typ, groesse, stufe, pruefsumme, UNIQUE (sitzung_id, datei))
```

Vier Vertraulichkeitsstufen: `oeffentlich`, `intern`, `vertraulich`, `geheim`.
Was eine Rolle nicht sehen darf, wird nicht ausgegraut — es steht gar nicht
erst in ihrem Zustand und verlässt den Server nie.

`pruefsumme` ist der SHA-256 der Datei beim Einlesen und wird vor **jeder**
Auslieferung erneut geprüft. Weicht sie ab, wurde die Datei unter dem
laufenden System ausgetauscht, und es wird nicht ausgeliefert.

Ein Zugriffsprotokoll gibt es hier bewusst nicht: wer wann welche Unterlage
geöffnet hat, steht als `unterlage_geoeffnet` beziehungsweise
`unterlage_verweigert` in der Ereigniskette. Nur sie ist fälschungssicher.

---

## Was im Arbeitsspeicher lebt und nicht in der Datenbank

Nicht alles gehört gespeichert. Diese Dinge sind bewusst flüchtig:

| Sache | Warum flüchtig |
|---|---|
| **Marke** der gepufferten Stimme | wäre bei geheimer Wahl ein Bindeglied zur Person |
| **Freigabe** einer Unterlage (30 s) | eine weitergegebene Marke soll nichts wert sein |
| Belegung eines Platzes (`belegt`) | ein Gerät ist keine Anwesenheit; nach dem Neustart meldet es sich neu an |

Nach einem Neustart stellt der Server aus der Ablage wieder her: Sitzungs- und
Punktzustand, Redeliste, laufende Abstimmung samt „wer hat abgestimmt", die
aktive Sitzungsleitung (aus der Kette). Die Marken sind dann weg — ein Gerät,
das seine Stimme nachreicht, bekommt „schon abgestimmt" statt einer
Bestätigung, und das ist die ehrliche Antwort: gezählt ist gezählt, doppelt
wird nie.

---

## Noch nicht gebaut

Diese Objekte stehen in den Leitlinien, existieren aber **nicht** als Tabelle:

- `redebeitrag` — die Wortmeldung wird festgehalten, der tatsächlich
  gesprochene Beitrag mit Anfang und Ende noch nicht als eigenes Objekt.
- `transkript_segment` und `transkript_korrektur` — das Transkript ist der
  nächste Schritt nach dem Ton.
- Alles zu Ton, Bildregie und Dolmetschen.
