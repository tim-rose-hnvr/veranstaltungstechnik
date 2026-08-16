# Öffentliche Webseite

`index.html` ist der Marketingauftritt. Eine Datei, keine externen Abrufe, kein
Build-Schritt — dieselbe Regel wie im Saal, damit sie überall läuft.

**Sie gehört nicht zum Saalsystem.** Der Go-Server im Wurzelverzeichnis wird
davon nicht berührt; hier liegen nur statische Dateien.

## Wix Headless

Im Konto liegt das Projekt **Saalwerk**:

| | |
|---|---|
| metaSiteId | `6f8c3073-d3df-43fb-96f2-7f496bc1e2ab` |
| siteId | `61a05f2d-cac7-48fb-8790-ba4ea76cb195` |
| Dashboard | https://manage.wix.com/dashboard/6f8c3073-d3df-43fb-96f2-7f496bc1e2ab |

Wix-managed Headless hostet statische Dateien mit CDN und SSL. Der Saalserver
läuft weiterhin auf eigener Hardware — Wix kann keinen Go-Prozess, kein
PostgreSQL und kein UDP zur Kamera betreiben.

## Veröffentlichen

Beide von Wix dokumentierten Wege legen dabei **ein eigenes Projekt** an; das
oben angelegte lässt sich damit nicht als Ziel wählen.

**Ohne Terminal:** [wix.com/headless/drop](https://www.wix.com/headless/drop)
öffnen, den Ordner `webseite/` hineinziehen. Ergebnis ist eine Live-Adresse auf
`*.wix-site-host.com`. Danach anmelden, damit die Seite dem Konto gehört.

**Mit Terminal** (nötig, wenn später Servercode dazukommt):

```sh
cd webseite
npm create @wix/new@latest init   # legt Projekt und wix.config.json an, fragt nach Login
npx wix release                   # veröffentlicht den Inhalt
```

In `wix.config.json` muss `site.outputDirectory` auf `"."` stehen — die Seite
hat keinen Build-Schritt, `index.html` liegt direkt im Ordner.

**Wenn das angelegte Projekt das Ziel bleiben soll:** im Dashboard unter den
Headless-Einstellungen einen OAuth-Client anlegen und dessen ID zusammen mit
der `siteId` von oben in eine selbst geschriebene `wix.config.json` eintragen.
Sonst bleibt das Projekt ungenutzt und kann gelöscht werden.

## Vor der Veröffentlichung zu erledigen

- **Impressum und Datenschutzerklärung** fehlen. In Deutschland ist beides
  Pflicht, sobald die Seite öffentlich erreichbar ist.
- **Kontaktadresse** steht als `kontakt@example.com` im Quelltext. Ich habe
  bewusst keine echte Adresse eingesetzt — eine öffentlich sichtbare Mailadresse
  ist eine Entscheidung, die du triffst, nicht ich.
- **Sprache und Region** des Projekts stehen auf Englisch, USA, Dollar. Umstellen
  im Dashboard unter Einstellungen → Business Info.
- **Produktname** ist ein Arbeitstitel. Vor der Veröffentlichung auf Marke und
  freie Domain prüfen.
- **Inhalt gegenlesen.** Die Seite sagt ausdrücklich, dass das System in
  Entwicklung ist und Pilotpartner gesucht werden. Das ist der Stand — wenn du
  anders auftreten willst, ändere den Abschnitt „Stand", nicht die Wahrheit.
