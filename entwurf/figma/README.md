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
| Fertig | 4 Variablensammlungen, 48 Variablen, 12 Textstile, 11 Seiten, Titelblatt |
| Vorbereitet | `02-fundament.js`, `03-platz.js` |
| Offen | Knopf, Wahlknopf, Statusband, Karte, Stufen-Marke, TOP-Zeile; die vier Oberflächen |

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

## Ungeprüft

`02-fundament.js` und `03-platz.js` sind **nicht gelaufen** — die Verbindung
riss ab, bevor sie an der Reihe waren. Sie stehen auf Mustern, die in
derselben Datei schon durchliefen (Variablen, Textstile, Titelblatt), aber
erwarte beim ersten Lauf Nachbesserung. Insbesondere `combineAsVariants` und
das Rastern danach sind erfahrungsgemäß die Stellen, an denen es hakt.
