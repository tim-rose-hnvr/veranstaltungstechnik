# Bedienung

Wie sich das System bedienen lässt — auf den vier Oberflächen, die es gibt.
Diese Datei beschreibt die **Absicht**. Was davon gebaut ist, steht bei jedem
Punkt ausdrücklich dabei; was fehlt, ebenso.

---

## Der Satz, aus dem die Bedienung folgt

**Die Oberfläche entscheidet nichts. Der Platz entscheidet.**

Das ist Leitprinzip 1 (*Person ≠ Platz ≠ Gerät*), zu Ende gedacht für die Frage
„was darf welches Gerät". Ein iPad am Vorsitzplatz darf, was der Vorsitz darf.
Dasselbe iPad, an Platz 7 gelegt, darf, was Platz 7 darf. Ein privates iPhone
darf nie mehr als die Begleitrolle — nicht weil es ein iPhone ist, sondern weil
es an keinem Platz steht.

Daraus folgt für jede der vier Oberflächen dieselbe Regel: **sie zeigt an, sie
prüft nicht.** Jede Rechtefrage beantwortet der Kern, und die Antwort kommt
fertig als Liste `darf` im Zustand. Eine Oberfläche, die selbst entscheidet,
wäre ein zweiter Regelsatz neben dem ersten — und zwei Regelsätze laufen
auseinander.

---

## Die vier Oberflächen

| | Wer bedient sie | Wo | Darf | Darf nie |
|---|---|---|---|---|
| **iPad** — Sprechstelle | Teilnehmende, Sitzungsleitung | fest am Platz | alles, was der Platz darf | Rechte selbst entscheiden |
| **iPhone** — Namensschild | niemand | fest am Platz, quer | nur anzeigen | bei geheimer Wahl etwas über die Stimme verraten |
| **iPhone** — Begleiter | Teilnehmende | privates Gerät | Unterlagen, Wortmeldung, Untertitel | **Mikrofon, Stimmabgabe** |
| **Web** — Browser | wie oben, je nach Platz | beliebig | was der Platz hergibt | mehr als der Platz hergibt |
| **Server** — Leitstand | Technik, Schriftführung | Nebenraum, Rand, Laptop | einrichten, prüfen, eingreifen | während einer Abstimmung eingreifen |

„Web" ist dabei keine eigene Rolle, sondern der Weg: heute **ist** der Browser
die Sprechstelle. Eine native App kommt erst, wenn Hintergrundton, Kiosk-Betrieb
und App Attest sie erzwingen. Bis dahin ist „iPad-App" ein iPad im Kiosk-Browser
und „iPhone-App" ein iPhone im Browser — mit denselben Schirmen.

---

## iPad — die Sprechstelle

### Die Lage

Das Gerät liegt auf dem Tisch vor jemandem, der **nicht wegen der Technik hier
ist**. Blickabstand 50 bis 70 Zentimeter, aber meistens schaut die Person gar
nicht hin — sie hört zu. Das Gerät muss deshalb in einer halben Sekunde zwei
Fragen beantworten:

1. Bin ich gerade dran?
2. Was ist der Punkt, über den geredet wird?

Alles andere ist nachrangig und darf die beiden nicht stören.

### Die Bedienlogik: ein Schirm, eine große Handlung

Es gibt zu keinem Zeitpunkt zwei gleichwertige große Knöpfe. Der Zustand
bestimmt, welche eine Handlung groß ist — der Rest wird klein oder verschwindet:

| Lage | Die große Handlung |
|---|---|
| nicht angemeldet | **Hier sitzen** (PIN) |
| angemeldet, Sitzung läuft nicht | keine — nur die Lage steht da |
| Sitzung läuft, kein Wort | **Wort melden** |
| gemeldet | **Meldung zurückziehen** |
| Wort erteilt | **Mikrofon an** |
| Mikrofon offen | **Mikrofon aus** |
| Abstimmung läuft | Vollbild, drei Flächen: Ja, Nein, Enthaltung |
| führender Platz verwaist | **Sitzungsleitung übernehmen** |

Die Liste ist keine Gestaltungsfrage. Sie fällt aus `ich.darf` heraus — was der
Kern nicht erlaubt, steht nicht da. Eine ausgegraute Schaltfläche ist eine
Behauptung über Rechte und deshalb verboten; entweder die Handlung ist möglich,
oder sie ist nicht sichtbar.

### Die Schirme

1. **Saalplan** — der Platz im Raum, nicht eine Liste. Wer auf Platz 7 sitzt,
   findet Platz 7 dort, wo er im Saal liegt. Fehlt die Sitzordnung (`reihe`,
   `spalte`), fällt die Anzeige auf eine Kachelreihe zurück: ein geratener Plan
   wäre schlimmer als keiner.
2. **Mein Platz** — die dominante Karte mit der einen Handlung.
3. **Tagesordnung** — welcher Punkt läuft, was kommt.
4. **Sitzungsmappe** — nur die Unterlagen, die diese Rolle sehen darf. Was sie
   nicht sehen darf, ist nicht ausgegraut, sondern **nicht vorhanden**: es
   verlässt den Server nie.
5. **Abstimmung** — als Vollbild, das alles andere verdeckt. Wer abstimmt, soll
   nichts anderes tun können.

### Was das iPad nie tut

- **Es entscheidet keine Rechte.** Es zeigt, was `darf` sagt.
- **Es zeigt keine Zwischenstände einer laufenden Abstimmung.** Ein
  Zwischenstand beeinflusst die noch Unentschiedenen.
- **Es verliert keine Stimme.** Die Stimme wird zuerst auf dem Gerät gesichert,
  dann geschickt; erst die Bestätigung des Servers löscht den Puffer. Reißt die
  Verbindung, steht auf dem Schirm, dass die Stimme gesichert ist und
  nachgereicht wird — und das Gerät darf liegen bleiben.

### Betriebliches, das zur Bedienung gehört

- **Ladegrenze 80 %.** Dauerladung auf 100 % zerstört iPad-Akkus binnen eines
  Jahres. Das ist eine Einrichtungsfrage, aber sie entscheidet, ob die Geräte
  nach zwei Jahren noch durch eine Sitzung kommen.
- **Kein NFC.** Ein iPad kann keine Karte und kein Telefon lesen. Anmeldung
  daher über PIN am Platz.
- **Kiosk.** Zwischen den Sitzungen zeigt das Gerät den Saalplan und sonst
  nichts — kein Zurück in einen Browser, keine andere App.

**Stand:** gebaut (`web/index.html`), einschließlich Saalplan, Mappe, Abstimmung
als Vollbild und Offline-Puffer. Kiosk und Ladegrenze sind Einrichtung, nicht
Code.

---

## iPhone — Namensschild

### Die Lage

Quer am Platz, fest montiert, von außen lesbar. Es ist eine **Anzeige, kein
Bedienteil**. Niemand meldet sich daran an, niemand tippt darauf.

### Was darauf steht

- Name und Funktion der Person am Platz
- Der Zustand, den der Saal sehen soll: spricht, hat das Wort, gemeldet
- Bei nicht besetztem Platz: die Platznummer, sonst nichts

### Was nie darauf steht

- **Bei geheimer Wahl nichts über die Stimme.** Nicht ob, nicht wie. Die
  Zuordnung Stimme→Person darf nirgends existieren, und ein Schild, das im
  richtigen Moment „hat abgestimmt" zeigt, ist ein Seitenkanal: wer den Saal
  filmt, kann die Reihenfolge rekonstruieren.
- Keine Unterlagen, keine Vertraulichkeitsstufen.

**Stand:** gebaut (`web/namensschild.html`). Die Abstimmungsregel hält von
Bauart wegen: das Schild liest den Abstimmungszustand gar nicht, es kennt nur
`mikro`, `hat_wort` und `belegt`. Das ist die stärkste Form der Zusage — was
nicht gelesen wird, kann nicht durchsickern. Ein Test hält das jetzt fest
(`TestNamensschildKenntDieAbstimmungNicht`): er schlägt an, sobald die Seite
`abstimmung`, `abgegeben`, `geheim`, `wahl` oder `stimme` anfasst.

---

## iPhone — der Begleiter auf dem privaten Gerät

### Die Lage

Das eigene Telefon der Teilnehmenden. Es steht an keinem Platz, gehört dem
Haus nicht und kann morgen verloren gehen.

### Die Regel, aus der alles folgt

**Nie Mikrofon, nie Stimmabgabe.** Ein privates Gerät bekommt ausschließlich
die Begleitrolle:

- Unterlagen lesen (mit persönlichem Wasserzeichen, Stufe wie am Platz)
- Wortmeldung setzen und zurückziehen
- Untertitel mitlesen

Der Grund ist nicht Misstrauen, sondern Zurechenbarkeit: eine Stimme muss einem
Platz zuzuordnen sein, und ein Platz ist ein Möbelstück im Saal, kein Gerät in
einer Tasche.

### Anmeldung

Über den **NFC-Tag am Platz**, gelesen vom iPhone. Das ist die einzige Stelle,
an der NFC in diesem System vorkommt — passiv am Platz, aktiv gelesen vom
Telefon der Person. Ein iPad könnte das nicht.

**Stand:** **nicht gebaut.** Es gibt keine Begleitrolle, keinen NFC-Weg und
keine Trennung zwischen Platzgerät und Privatgerät in der Oberfläche. Die Rolle
`gast` im Kern kommt dem am nächsten, meint aber etwas anderes.

---

## Web — der Browser

Heute ist der Browser der Träger aller Oberflächen oben. Zusätzlich gibt es
zwei Dinge, die nur im Browser Sinn ergeben:

### Zuschaltung von außen

Wer nicht im Saal ist, bekommt Bild und Ton und — je nach Satzung — Rede- und
Stimmrecht. Die Tonstrecke dorthin braucht **Mix-Minus**: eine Summe ohne die
eigene Quelle, sonst hört sich die Zuschaltung verzögert selbst und schaukelt
sich auf. Das Regelwerk dafür steht (`intern/ton`), die Signalverarbeitung
nicht.

**Stand:** **nicht gebaut.**

### Der Prüfstand

`/emulator` zeigt alle Plätze nebeneinander und **gibt die PINs im Klartext
preis**. Er gehört deshalb nie in eine Konfiguration, die im Saal läuft, und ist
über einen eigenen Schalter gesperrt.

**Stand:** gebaut (`web/emulator.html`, `web/testumgebung.html`).

---

## Server — der Leitstand

**Das ist die größte Lücke.** Der Server ist ein Ubuntu ohne Oberfläche. Was es
heute gibt, ist ein Selbsttest (`/vorabcheck`) und die Prüfstelle. Es gibt
**kein Einrichten, keinen laufenden Betrieb, kein Nachher**.

### Die Bedienlogik: vorher führend, währenddessen stumm

Die Technik hat drei Zeiten, und sie verlangen entgegengesetzte Oberflächen.

#### Vorher — einrichten und bestücken

| Schirm | Was darauf geschieht | Stand |
|---|---|---|
| **Saal** | Plätze anlegen, Sitzordnung (`reihe`, `spalte`), Kameras, Presets je Platz lernen | fehlt — heute `saal.json` von Hand |
| **Sitzung** | Teilnahmen zuordnen, Rollen setzen, Tagesordnung, Sitzungsmappe mit Stufen | fehlt — heute `sitzung.json` von Hand |
| **PIN-Ausgabe** | PINs erzeugen und **ausdrucken** — sie stehen nirgends lesbar, auch nicht für die Technik | fehlt ganz |
| **Vorabcheck** | Selbsttest 15 Minuten vorher: Kette nachrechnen, jede Kamera einmal anfahren, Besetzung prüfen, Beschlussfähigkeit hochrechnen, Einmessung bewerten | **gebaut** |

Die PIN-Ausgabe ist die unangenehmste Lücke: heute gibt es keinen Weg, eine PIN
zu einer Person zu bringen, außer sie in der Prüfstelle abzulesen — und die
darf im Saal nicht laufen. Ein Ausdruck, der je Platz einen abtrennbaren
Abschnitt trägt, ist der naheliegende Weg.

#### Währenddessen — zusehen, selten eingreifen

Der Leitstand ist im laufenden Betrieb **das ruhigste Fenster im Haus**. Er
zeigt:

- Saalplan mit offenen Mikrofonen und der Summendämpfung des Automixers
- Redeliste
- Kamerazustand je Kamera, samt „stumm" wenn eine nicht antwortet
- Die Kette: wächst sie, ist alles gut

Eingriffe sind möglich, aber teuer gemacht — und **während einer laufenden
Abstimmung gesperrt**. Eine Kamerafahrt mitten im Beschluss ist ein Eingriff,
den niemand angeordnet hat.

Die **Eskalationsleiter** gehört hierher: sechs Stufen automatischer
Gegenmaßnahmen bei Störungen, ab Stufe 4 nur mit ausdrücklicher Bestätigung
eines Menschen. **Nicht gebaut.**

#### Nachher — schließen und belegen

| Schirm | Was darauf geschieht | Stand |
|---|---|---|
| **Protokollentwurf** | fällt aus der Kette an, gegliedert nach Tagesordnungspunkten | **gebaut** (`/protokoll.md`) |
| **Siegel** | Kette abschließen und unterschreiben, Prüfbericht ansehen | **gebaut** (`/siegel.json`, `POST /siegel`) |
| **Ausgabe** | Protokoll, Beschlüsse, Anwesenheit — nach OParl zurückschreiben, wo es ein Ratsinformationssystem gibt | fehlt |

---

## Was über alle vier hinweg gilt

### Farbe bedeutet etwas

Vier Signalfarben sind im Saal belegt und dürfen für nichts anderes stehen:

| Farbe | Bedeutung |
|---|---|
| Rot | Mikrofon offen |
| Blau | hat das Wort |
| Grün | frei |
| Bernstein | Achtung, Widerspruch gegen die Aufzeichnung |

Die Hausfarbe hält sich davon fern (Tintengrün). Wer im Saal Farbe sieht, soll
daraus etwas ableiten können — die Marke darf da nicht mitreden. Zustand hängt
nie allein an der Farbe: jede Kachel trägt zusätzlich Text.

### Ein Ausfall kostet Komfort, nie Daten

Auf jeder Oberfläche, die etwas absendet, gilt derselbe Ablauf: erst auf dem
Gerät sichern, dann schicken, erst löschen, wenn es angekommen ist. Gebaut für
**Stimme und Wortmeldung**.

Die beiden Puffer sind verschieden gebaut, weil die Sache verschieden ist. Die
Stimme braucht eine **Marke**: sie darf genau einmal zählen, und nur ein
Einmalwert macht die Wiederholung unterscheidbar von einer zweiten Stimme. Die
Wortmeldung braucht keine — der Kern nimmt je Platz ohnehin nur eine offene
Meldung an und antwortet auf eine Wiederholung mit Erfolg. Das Gerät darf also
blind nachreichen.

Beide verfallen, wenn sie nicht mehr passen, und sagen es: die Stimme, wenn die
Abstimmung geschlossen ist; die Wortmeldung, wenn der Tagesordnungspunkt
gewechselt hat. Wer sich zu Punkt 3 gemeldet hat, meldet sich nicht
stillschweigend zu Punkt 4. Und beide sind an den **Platz** gebunden: meldet
sich am selben Gerät jemand anderes an, werden sie verworfen statt unter
fremdem Namen gezählt.

### Die Oberfläche prüft nicht

Jede Rechtefrage beantwortet `intern/kern`. Die Oberfläche bekommt `darf` als
fertige Liste und richtet sich danach. Ein Befehl ohne Recht darf abgeschickt
werden — er wird im Kern abgewiesen, und das ist die Prüfung.

---

## Was fehlt, geordnet nach Schaden

1. **Leitstand für den Betrieb** — ohne ihn ist das System für einen Techniker
   nicht bedienbar, nur für jemanden, der JSON-Dateien schreibt.
2. **PIN-Ausgabe** — es gibt keinen sicheren Weg, eine PIN zur Person zu bringen.
3. **Einrichtung von Saal und Sitzung** — heute von Hand in JSON.
4. **Begleitrolle auf dem privaten Gerät** — samt NFC am Platz.
5. **Eskalationsleiter** — sechs Stufen, ab Stufe 4 mit Bestätigung.
6. **Zuschaltung** — braucht die Tonstrecke, die den echten Raum braucht.
