# Entscheidungsprotokoll

Alles hier ist **entschieden**. Diese Punkte werden nicht erneut zur Diskussion gestellt und nicht als Frage an den Nutzer zurückgegeben.

Wenn eine Entscheidung technisch nicht haltbar ist, das sagen und begründen — aber nicht als offene Frage behandeln.

---

## Architektur

| # | Frage | Entscheidung | Begründung |
|---|---|---|---|
| A1 | Sprache Backend | **Go** | ein Binary, gute Nebenläufigkeit, offline verteilbar |
| A2 | Datenbank | **PostgreSQL** | Transaktionen für die Ereigniskette, Replikation für Redundanz |
| A3 | Client zuerst | **HTML plus reines JavaScript**, kein Framework | Änderungen in Sekunden statt Build-Zyklen |
| A4 | Native iPad-App | **später**, erst für Hintergrund-Audio, Kiosk und App Attest | Xcode braucht einen Mac, blockiert den Anfang |
| A5 | Echtzeitkanal | **WebSocket**, voller Zustand je Nachricht | einfacher als Teiländerungen, Aussetzer heilen sich selbst |
| A6 | Medien später | **GStreamer** | SFU, Kompositor und NDI in einem Werkzeug |
| A7 | Server-Betriebssystem | **Ubuntu Server LTS**, ohne Oberfläche | fünf Jahre Pflege, aktuelle Medienbibliotheken |
| A8 | Container | **Podman**, nicht zwingend im Meilenstein 1 | rootless, ohne Dienst im Hintergrund |
| A9 | Zertifikate | **eigene CA über step-ca**, mTLS zwischen Diensten | funktioniert ohne Internet |

## Ausdrücklich verworfen

| # | Idee | Warum nicht |
|---|---|---|
| V1 | Node.js im Backend | Ein Binary ohne Laufzeitumgebung ist beim Kunden einfacher |
| V2 | vMix oder OBS im Produkt | Fremdsoftware beim Kunden, keine Kontrolle, Lizenzfragen |
| V3 | Teams Media Bot | braucht Azure und Admin-Freigabe im Tenant des Kunden — später, wenn ein zahlender Kunde es verlangt |
| V4 | Windows Server als Kern | Lizenzkosten, erzwungene Neustarts, schwer in abgeschotteten Netzen |
| V5 | macOS als Dauerserver | nach Stromausfall nicht headless entsperrbar, nicht klonbar |
| V6 | Cloud-Betrieb als Standard | widerspricht dem einzigen echten Verkaufsargument |
| V7 | React im ersten Client | Build-Schritt ohne Nutzen für den Zweck |
| V8 | Eigene Kryptografie | nie, unter keinen Umständen |
| V9 | Browser-Speicher-APIs | in der Zielumgebung nicht verlässlich |

## Fachliche Festlegungen

| # | Punkt | Festlegung |
|---|---|---|
| F1 | Identität | Der **Sitzplatz** trägt sie, nicht Person und nicht Gerät |
| F2 | Gleichzeitig offene Mikrofone | begrenzt, Wert aus der Einmessung; Vorgabe für Tests: 3 |
| F3 | Kamerapresets | liegen **in der Kamera**, der Server merkt sich nur die Nummer |
| F4 | Kameraausfall | ist kein Systemfehler; protokollieren und weiterlaufen |
| F5 | Ereignisprotokoll | Hash-Kette, nur anfügbar, keine Aktualisierung, kein Löschen |
| F6 | Zeitangaben | Millisekunden seit Sitzungsbeginn, eine Zeitachse für alles |
| F7 | Offline | Der Saal funktioniert ohne Internet. Keine externe Schriftart, kein CDN, kein Lizenzabruf |
| F8 | Verkabelung | Ein Cat6 je Platz, PoE. WLAN nur mobil und als Rückfall |
| F9 | NFC | iPads haben **keins**. Nur passiver Tag am Platz, gelesen vom iPhone des Teilnehmers |
| F10 | Laden | Ladegrenze 80 %, sonst sind die Akkus nach einem Jahr hin |
| F11 | Bildkacheln | In 16:9 gilt Kachelhöhe in Prozent = Kachelbreite in Prozent. Schwarze Balken sind gewollt |
| F12 | Bildwechsel | Mindeststandzeit 2 bis 3 Sekunden, Einwürfe unter 1,5 Sekunden ignorieren |
| F13 | Mix-Minus | von Anfang an im Audiorouting vorgesehen, nachträglich sehr teuer |
| F14 | Geheime Wahl | Zuordnung Stimme zu Person existiert nirgends, auch nicht im Log |
| F15 | Sitzungsleitung | genau eine aktiv, Übergabe nur ausdrücklich, mit Protokolleintrag |
| F16 | Aufzeichnungsfreigabe | je Person; wer widerspricht, wird von der Nachführung übersprungen |
| F17 | Automatische Eingriffe | während laufender Abstimmung gesperrt |
| F18 | Nachrüstung Fremdanlagen | über Dante oder AES67, nicht über Herstellerschnittstellen |
| F19 | Namensschild | E-Paper mit ESP32 als Zielbild, iPad mini als Zwischenlösung |

## Produkt und Vertrieb

| # | Punkt | Festlegung |
|---|---|---|
| P1 | Ausbaustufen | Lite, Normal, Premium, Enterprise |
| P2 | Rechtssicherheit | in **keiner** Stufe reduziert; gestaffelt wird nur Souveränität |
| P3 | Grenze Lite zu Normal | Physik: ohne Einmessung keine Saalbeschallung |
| P4 | Anpassung je Kunde | Gestaltungsdaten und Konfiguration, **nie** ein eigener Codestand |
| P5 | Öffentliche Webseite | darf gehostet sein; Kundendaten und Zuschaltung nicht |
| P6 | Preise auf der Webseite | keine |
| P7 | Produktname | **ConferenceCue** |

## Noch offen — hier darf gefragt werden

- Erster Pilotkunde und Termin
- Verteilung der nativen App: Apple Business Manager oder App Store
- Ob Dante schon in der ersten Version gebraucht wird
- Endgültiger Produktname
