# Projektanweisung — Sitzungssystem

Diese Datei gehört in die Projektanweisungen (Claude Projects) oder als `CLAUDE.md` ins Repository-Wurzelverzeichnis.

---

## Was gebaut wird

Ein Konferenz-, Abstimmungs- und Protokollsystem für Gremiensitzungen, Aufsichtsräte und Hauptversammlungen. iPads als Sprechstelle, ein Server im Raum, PTZ-Kameras mit automatischer Nachführung, Sitzungsmappe, Abstimmung, Transkript, Protokoll.

Betrieben wird es beim Kunden auf eigener Hardware. Kein Cloud-Zwang, keine Abhängigkeit vom Internet während einer Sitzung.

**Produktname:** ConferenceCue.

**Stand:** Kern, Kameranachführung, Redeliste, Tagesordnung, Abstimmung, Sitzungsmappe, Protokoll und Vorabcheck laufen und sind geprüft. Ton, Bildregie und Transkript sind nicht gebaut. Vorführbetrieb ohne Datenbank: `go run ./cmd/server -konfiguration vorfuehrung/config.yaml`.

---

## Leitprinzipien

Diese vier Sätze entscheiden fast jede Detailfrage. Bei Zweifeln hierauf zurückgehen.

1. **Person ≠ Platz ≠ Gerät.** Der Sitzplatz trägt die Identität: Nummer, Funktion, Stimmrecht, Kamerapreset. Personen werden je Sitzung zugeordnet, Geräte sind austauschbar. Ein defektes iPad wird ersetzt, ohne dass etwas neu eingerichtet wird.
2. **Der Saal hängt von nichts ab.** Kein Lizenzabruf im Netz, kein CDN, keine externe Schriftart, kein Analytics. Das System muss in einem abgeschotteten Netz vollständig funktionieren. Entwickle mit abgeschaltetem Internet, dann kann sich keine Abhängigkeit einschleichen.
3. **Ein Ausfall darf Komfort kosten, niemals Daten.** Drei Sekunden Tonlücke sind verzeihlich, eine verlorene Stimme nicht. Stimmen und Protokolleinträge werden auf dem Gerät signiert und gepuffert, bevor sie den Server erreichen.
4. **Ein Programm, keine Sonderzweige.** Jeder Kundenwunsch wird entweder ein allgemeines Merkmal oder er wird abgelehnt. Anpassung geschieht über Konfiguration und Gestaltungsdaten, nie über einen eigenen Codestand.

---

## Technikstack — entschieden, nicht neu zu diskutieren

| Bereich | Wahl |
|---|---|
| Kern | Go, WebSocket, PostgreSQL |
| Medien | GStreamer (SFU, Kompositor, NDI-Ein- und Ausgang), WebRTC |
| Kamerasteuerung | VISCA over IP, UDP 52381; ONVIF als Alternative |
| Tontransport | Opus über WebRTC im Raum, AES67/Dante zur Anlagentechnik |
| Client | zuerst Web-App; native iPadOS-App (SwiftUI) erst, wenn Hintergrund-Audio, Kiosk-Betrieb und App Attest gebraucht werden |
| Server-Betriebssystem | Ubuntu Server LTS, ohne Oberfläche, Container über Podman |
| Virtuelle Kamera | eigene CoreMediaIO-Erweiterung auf macOS |
| Zertifikate | eigene CA über step-ca, mTLS zwischen allen Diensten |

**Bewusst nicht verwendet:** vMix und OBS im Produkt (nur zum Lernen), Windows Server als Kern, macOS als Dauerserver, eigene Kryptografie, Browser-Speicher-APIs in Oberflächen-Entwürfen.

---

## Fundamente, die später nicht nachrüstbar sind

Diese sechs Punkte müssen von der ersten Zeile an stimmen. Alles andere ist additiv.

1. **Mandantenfähigkeit** — `organisation_id` in jeder Tabelle
2. **Ereignisprotokoll als Hash-Kette**, nur anfügbar, mit täglichem signiertem Abschluss — gebaut, siehe `intern/siegel`
3. **Eine Zeitachse je Sitzung** — Audio, Video, Transkript, Abstimmungen und Telemetrie in Millisekunden seit Sitzungsbeginn
4. **Offline-First im Client** — Gerät arbeitet weiter, wenn der Server weg ist, und synchronisiert nach. **Noch nicht gebaut**: der Client puffert nichts, eine Stimme bei getrennter Verbindung geht verloren. Braucht eine Entscheidung über Browser-Speicher.
5. **Rollen statt Gerätetypen**, Rechte zentral im Kern geprüft, nie in der Oberfläche
6. **Mix-Minus im Audiorouting** — getrennte Summen für Saal und Zuschaltung

---

## Fachwissen, das im Code steckt

Diese Regeln sind keine Meinungen, sondern Ergebnisse aus Akustik, Funktechnik und Bildmischung. Sie dürfen nicht wegoptimiert werden.

**Ton**
- Jede Verdopplung offener Mikrofone kostet 3 dB Reserve. Die Höchstzahl offener Mikrofone folgt aus der Einmessung, typisch 8.
- Mikrofon zur Saalbeschallung: unter 10 ms, sonst Kammfiltereffekte mit der echten Stimme.
- Dolmetschkanal zum Kopfhörer: unter 60 ms, weil der Zuhörer den Saal parallel hört.
- Zwei Sorten Rückkopplungsfilter: feste aus dem Ring-out, kurzlebige für neu auftretende. Maximal 6 bis 8 schmale Filter, sonst stirbt der Klang.
- Hochpass je Kanal bei 100 bis 120 Hz gegen Tischrumpeln.
- Ohne Mix-Minus entsteht zwischen Saal und Zuschaltung eine Schleife.

**Bild**
- In einem 16:9-Rahmen gilt: Kachelhöhe in Prozent = Kachelbreite in Prozent. Nur so bleibt jede Kachel selbst 16:9. Schwarze Balken sind gewollt, Strecken und Beschneiden nicht.
- Spaltenzahl nach Sprecherzahl: 1→1, 2→2, 3→2, 4→2, 5–6→3. Block zentriert, unvollständige Reihen zentriert.
- Mindeststandzeit 2 bis 3 Sekunden, Einwürfe unter 1,5 Sekunden ignorieren. Ohne Hysterese wird das Bild unbrauchbar.
- Über der Kachelgrenze: Totale statt immer kleinerer Kacheln.
- NDI voll braucht 100 bis 250 Mbit/s je Strom — PTZ-Kameras deshalb immer verkabelt. Endgeräte senden Video nur bei offenem Mikrofon.

**Recht und Verfahren**
- Abstimmung startet nur bei Beschlussfähigkeit. Quorum wird beim Start eingefroren.
- Bei geheimer Wahl darf die Zuordnung Stimme→Person nirgends existieren, auch nicht im Log oder auf dem Namensschild. Blind signiertes Einmal-Token.
- Sitzungsleitung ist ein Staffelstab: berechtigt sind viele, aktiv ist genau eine. Übergabe nur ausdrücklich, mit Protokolleintrag.
- Wer der Aufzeichnung widersprochen hat, wird von der Kameranachführung automatisch übersprungen.
- Nicht öffentlicher Tagesordnungspunkt: Stream und Aufzeichnung pausieren automatisch, gesteuert vom Sitzungszustand.
- Automatische Technikeingriffe sind während einer laufenden Abstimmung gesperrt.

**Geräte**
- iPads haben **kein NFC**. Ein iPad kann keine Karte und kein Telefon lesen. NFC funktioniert nur als passiver Tag am Platz, gelesen vom iPhone des Teilnehmers.
- Dauerladung auf 100 % zerstört iPad-Akkus binnen eines Jahres. Ladegrenze 80 %.
- Private Geräte bekommen nur die Begleitrolle: Dokumente, Wortmeldung, Untertitel. Nie Mikrofon, nie Stimmabgabe.

---

## Datenmodell in Kürze

Vollständig in `datenmodell.md`. Die tragenden Objekte:

`organisation` → `saal` → `platz` → `geraet`
`person` + `sitzung` + `rolle` → **`teilnahme`** (verbindet Person, Platz und Rolle für eine Sitzung)
`tagesordnungspunkt` → `wortmeldung` → `redebeitrag`
`abstimmung` → `stimme`
`ereignis` (Hash-Kette, nur anfügbar)
`transkript_segment` → `transkript_korrektur`

Zustandsketten:
- Teilnahme: `eingeladen → registriert → angemeldet → anwesend → abwesend`
- Sitzung: `vorbereitet → bereit → laufend ⇄ unterbrochen → geschlossen → archiviert`
- Wortmeldung: `gemeldet → erteilt → laufend → beendet | entzogen | zurueckgezogen`
- Abstimmung: `vorbereitet → laufend → ausgezaehlt → festgestellt | abgebrochen`

---

## Baureihenfolge

**Erledigt:** Kern (Sitzungszustand, Rollen, Rechte), Kamerasteuerung über VISCA, Sprechstelle, Redeliste, Abstimmung, Protokoll, Vorabcheck, Tagesordnung mit automatischer Aufzeichnungspause, Sitzungsmappe mit Vertraulichkeitsstufen.

**Jetzt:** Ton (Automixer, Mix-Minus, Einmessung) — braucht einen echten Raum, keinen Schreibtisch.

**Danach:** Transkript → Kompositor.

**Nicht zuerst:** Kompositor, Dolmetschen, Hauptversammlungsmodus, Teams-Media-Bot, eigene Hardware. Alles additiv, alles später.

Wer mit dem Kompositor anfängt, versandet.

---

## Arbeitsweise

- **Sprache Deutsch**, auch in Bezeichnern und Kommentaren, wo es die Domäne betrifft (`sitzung`, `wortmeldung`, `platz`). Technische Begriffe bleiben englisch, wo sie etabliert sind.
- **Nachfragen statt raten.** Wenn eine Anforderung mehrdeutig ist, eine Rückfrage stellen, keine Annahme einbauen.
- **Keine erfundenen Bibliotheken oder API-Aufrufe.** Wenn eine Schnittstelle unsicher ist, das sagen und nachschlagen lassen.
- **Echte Dateien statt Skizzen.** Wenn Code entsteht, dann lauffähig, mit Fehlerbehandlung und Tests für die Regeln oben.
- **Widersprich, wenn etwas falsch ist.** Die Fachregeln in diesem Dokument sind teuer erarbeitet — aber wenn ein Punkt technisch nicht haltbar ist, sag es, statt ihn umzusetzen.
- **Kurze Antworten.** Ergebnis zuerst, Begründung nur, wo sie etwas ändert.

---

## Glossar

| Begriff | Bedeutung |
|---|---|
| Synoptik | Saalplan, in dem Mikrofone durch Antippen geschaltet werden. Branchenstandard. |
| Ring-out | Verstärkung schrittweise anheben, bis Rückkopplung entsteht, Frequenz filtern, wiederholen. Ergibt die Reserve in dB. |
| Automixer | Regelt offene Mikrofone nach Anzahl, senkt die Gesamtverstärkung entsprechend. |
| Mix-Minus | Eine Summe, aus der die eigene Quelle entfernt ist. Verhindert Schleifen zur Zuschaltung. |
| Preset | In der Kamera gespeicherte Position für einen Sitzplatz. Liegt in der Kamera, nicht nur im Server. |
| Vorabcheck | Automatischer Selbsttest aller Plätze 15 Minuten vor Sitzungsbeginn. |
| Eskalationsleiter | Sechs Stufen automatischer Gegenmaßnahmen; ab Stufe 4 nur mit Bestätigung. |
| OParl | Offener Standard für Ratsinformationssysteme. Tagesordnung lesen, Beschlüsse zurückschreiben. |
| Krypto-Löschung | Löschen durch Vernichten des Schlüssels — wirkt auch auf Kopien und Offline-Geräten. |

---

## Offene Entscheidungen

- Erster Pilotkunde und Termin
- Verteilung der nativen App: Apple Business Manager oder App Store
- Ob Dante-Anbindung in der ersten Version gebraucht wird oder erst zur Nachrüstung
