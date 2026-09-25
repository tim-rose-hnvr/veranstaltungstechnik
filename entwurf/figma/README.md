# Figma-Designsystem

Die Bauschritte für die Figma-Datei
[ConferenceCue — Designsystem](https://www.figma.com/design/cKPTYwkOrRq2nsy1oyiUx2).

Diese Skripte laufen im Figma-Plugin-Kontext (`use_figma`), nicht in Node und
nicht im Browser. Sie stehen hier, weil sie die Gestaltungsentscheidungen
tragen und eine abgerissene Verbindung sie sonst mitnimmt.

## Stand

| | |
|---|---|
| Datei | `cKPTYwkOrRq2nsy1oyiUx2` |
| Fundament | 4 Variablensammlungen, 48 Variablen, 13 Textstile, Modus Hell und Dunkel |
| Bausteine | Knopf, Platz, Wahlknopf, Stimmkarte, Stufe, TOP-Zeile, Redeliste-Zeile, Unterlage-Zeile, Statusband |
| Oberflächen | 27 Schirme, siehe unten |
| Offen | Leitstand: Protokoll und Ausgabe (OParl) als eigene Schirme |

### Die Oberflächen

| Seite | Schirme | Format |
|---|---|---|
| Sprechstelle | am Platz · gemeldet · hat das Wort · spricht · Verbindung weg · Anmeldung (PIN) · zwei im Modus Dunkel | iPad quer, 1194 × 834 |
| Abstimmungsschirm | Stimmzettel · Stimme gezählt · Verbindung weg · Leitung · Ergebnis | iPad quer |
| Namensschild | am Platz · hat das Wort · spricht · frei · keine Verbindung | iPhone quer, 852 × 393 |
| Begleiter | Anmeldung über NFC · Mappe · Wortmeldung · Untertitel · Abstimmung läuft | iPhone hoch, 393 × 852 |
| Browser | Zuschaltung · Zuschaltung bei nicht öffentlichem Punkt | 1440 × 900 |
| Leitstand | Saal · Sitzung · PIN-Ausgabe · Vorabcheck · Betrieb (Abstimmung läuft) · Siegel | 1440 × 900 |

Was davon **Entwurf für Ungebautes** ist, steht in `bedienung.md` unter
„Stand": Begleiter, Zuschaltung, der ganze Leitstand außer Vorabcheck und
Siegel. Die Schirme zeigen die Zielgestalt; sie behaupten nichts, was der Code
nicht hält — wo etwas noch fehlt (Signatur auf dem Gerät, OParl), steht es
nicht auf dem Schirm.

### Entscheidungen, die in den Schirmen stecken

- **Eine große Handlung je Zustand.** Die Sprechstelle hat nie zwei
  gleichwertige große Knöpfe.
- **Prüfergebnisse tragen keine Signalfarbe.** Fehler, Hinweis, in Ordnung
  steigen im Leitstand mit der Tintendichte — Rot bleibt dem Mikrofon.
- **Gesperrt heißt: nicht da.** Während einer Abstimmung zeigt der Leitstand
  keine ausgegrauten Eingriffe, sondern ein Band, das sagt, was wartet.
- **Geheime Wahl bis in den Leitstand:** die Kette zeigt „Stimme abgegeben
  (7.)" ohne Platz, der Schirm nach der Abgabe zeigt die Wahl nicht.
- **Die PIN steht nur auf Papier.** Der Kern hält einen bcrypt-Hash; die
  Vorschau zeigt Punkte.
- **Zuschaltung ist ein eigener Platz** (Z1), nicht ein Gerät an einem
  Saalplatz.

`zustand.json` hält jede Knoten- und Variablenkennung. Ohne sie müsste man
raten, und geraten wird hier nichts.

## Die Gestaltungsrichtung

Die Oberfläche gehört zu dem, was das System herstellt: einer **Niederschrift**.
Papierton statt Konsolengrau.

**Die Schriftzuordnung trägt eine Aussage.** Source Serif steht ausschließlich
für Texte, die rechtlich binden — Beschluss und Tagesordnungspunkt. Plex Sans
bedient, Plex Mono zählt. Wer eine Serife sieht, weiß, dass dort etwas
Verbindliches steht.

**Farbe bedeutet im Saal etwas.** Vier Signalfarben sind belegt und dürfen für
nichts anderes stehen:

| Farbe | Bedeutung |
|---|---|
| `#C8102E` | Mikrofon offen |
| `#1D4E89` | hat das Wort |
| `#3F6B4A` | frei |
| `#8A5A00` | Achtung, Widerspruch gegen die Aufzeichnung |

Die Hausfarbe hält sich davon fern: Tintengrün `#14453F`. Der erste Entwurf
hatte Siegelrot, und auf dem Bildschirm stand es dann neben „Mikrofon offen" —
verwechselbar. Im Saal bedeutet Rot genau eine Sache. **Die Marke hat zu
weichen, nicht das Signal.**

Zustand hängt nie allein an der Farbe: jede Kachel trägt zusätzlich Text.

## Ausführen

Jede Datei ist der vollständige Rumpf eines `use_figma`-Aufrufs — Inhalt
übergeben, Dateikennung dazu, fertig. Der Reihe nach, die Nummern sind die
Reihenfolge. Jedes Skript gibt die erzeugten Knotenkennungen zurück; die
gehören nach dem Lauf in `zustand.json`.

Vorher gilt die Regel des Figma-Werkzeugs: die `figma-use`-Anleitung laden,
sonst laufen die üblichen Fallen auf.

## Fallen, die hier aufliefen

- `resize()` auf einem Textknoten setzt `textAutoResize` zurück — erst
  `resize`, dann `textAutoResize = 'HEIGHT'`.
- Schattenausbreitung zeichnet Figma nur mit `clipsContent = true`.
- Eine Farbe erneut an dieselbe Variable binden, mit `color` 0,0,0 als
  Platzhalter, lässt den Rohwert Schwarz stehen — und genau den zeigt der
  Renderer. Beim Binden den aufgelösten Wert mitgeben.
- `text/auf-marke` war im Modus Dunkel weiß, während die gefüllten Flächen hell
  werden: 1,1 : 1 auf dem Sperrband. Jetzt dunkel, jede Paarung ≥ 5,5 : 1.
  **Derselbe Fehler steckt in `web/index.html`** (weiß auf `--an` im Dunkeln,
  etwa 3,3 : 1) — dort nicht angefasst, weil der Code in diesem Schritt
  bleiben sollte.
- Ein fehlgeschlagener `use_figma`-Lauf rollt vollständig zurück.
