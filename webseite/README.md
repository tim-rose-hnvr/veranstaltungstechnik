# Webseite

Die Produktseite zu ConferenceCue. Sechs Seiten, eine gemeinsame `stil.css`,
sonst nichts: kein Aufbauschritt, keine Abhängigkeit, kein CDN, keine externe
Schriftart, kein Analytics — dieselbe Regel wie im Saal.

| Datei | Inhalt |
|---|---|
| `index.html` | Start: die Lücke zwischen Saaltechnik und Board-Portal, die Leitsätze |
| `system.html` | Bestandteile, Fachregeln, Technikstack |
| `einsatz.html` | sechs Räume, vier Ausbaustufen, warum PTZ verkabelt wird |
| `sicherheit.html` | Betrieb im eigenen Haus, Ereigniskette, geheime Wahl, Vorabcheck |
| `stand.html` | was läuft und was nicht — bewusst unbequem |
| `kontakt.html` | Vorführung, Pilotpartner |
| `bilder/` | sechs Symbolbilder (JPEG, je unter 350 KiB) |

## Bilder und Schaubilder

Die Fotos sind **computergenerierte Symbolbilder**, keine Referenzfotos — die
Bildunterschriften sagen das ausdrücklich. Sie zeigen keine echten Kunden,
keine echten Räume und keine erkennbaren Personen realer Herkunft. Vor der
Veröffentlichung dürfen sie bleiben, sollten aber gegen Fotos der ersten
echten Installation getauscht werden, sobald es eine gibt.

Die Schaubilder (Signalfluss auf `system.html`, Ereigniskette auf
`sicherheit.html`) sind eingebettetes SVG in Seitenfarben — `currentColor`
als Stiftfarbe, das Akzentrot nur für die eine Aussage des Bilds. Auf
schmalen Schirmen rollen sie seitlich, statt unlesbar zu schrumpfen.

## Ansehen

```sh
python3 -m http.server 8099 --directory webseite
```

## Bearbeiten

Kopfleiste und Fußzeile stehen in jeder Datei einzeln. Das ist Absicht: eine
Seite mit sechs Dateien braucht keinen Erzeuger, und jede Datei bleibt für sich
lesbar und änderbar. Wer die Navigation ändert, ändert sie sechsmal.

Die aktive Seite trägt in der Navigation `aria-current="page"`.

## Vor der Veröffentlichung

- Impressum und Datenschutzerklärung fehlen und sind **rechtlich verpflichtend**.
- Die Kontaktadresse steht als `kontakt@example.com` in `kontakt.html` und muss
  ersetzt werden.
