package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/ton"
)

// Konfiguration ist der Inhalt von config.yaml.
type Konfiguration struct {
	Datenbank          string `yaml:"datenbank"`
	Adresse            string `yaml:"adresse"`
	SaalDatei          string `yaml:"saal_datei"`
	SitzungsDatei      string `yaml:"sitzungs_datei"`
	MaxOffeneMikrofone int    `yaml:"max_offene_mikrofone"`
	// EinmessungReserveDB ist die Reserve aus dem Ring-out in dB. Ist sie
	// gesetzt, folgt die Höchstzahl offener Mikrofone aus ihr — eine
	// Konfiguration, die mehr verlangt, als der Saal hergibt, wird beim
	// Start abgewiesen. Null: es gibt keine Einmessung, die Höchstzahl ist
	// gesetzt statt gemessen.
	EinmessungReserveDB float64 `yaml:"einmessung_reserve_db"`
	KameraZeitlimitMs   int     `yaml:"kamera_zeitlimit_ms"`

	// KameraAttrappe lässt simulierte Kameras auf den Adressen aus saal.json
	// hören. Der Weg bleibt derselbe — Rahmen, UDP, Quittung —, nur steht am
	// Ende keine Optik.
	KameraAttrappe bool `yaml:"kamera_attrappe"`
	// SiegelSchluessel ist der Ed25519-Schlüssel, mit dem die Ereigniskette
	// abgeschlossen wird. Fehlt die Datei, wird sie beim Start angelegt.
	SiegelSchluessel string `yaml:"siegel_schluessel"`
	// SiegelUhrzeit ist die Ortszeit des täglichen Abschlusses, "23:55".
	// Leer: nur beim Herunterfahren und auf Anforderung.
	SiegelUhrzeit string `yaml:"siegel_uhrzeit"`
	// Emulator schaltet /emulator frei. Die Seite braucht die PINs im
	// Klartext, deshalb ist das eine ausdrückliche Entscheidung und kein
	// Nebeneffekt des Vorführbetriebs.
	Emulator bool `yaml:"emulator"`
}

// KonfigurationLesen liest config.yaml und prüft die Pflichtangaben.
func KonfigurationLesen(pfad string) (Konfiguration, error) {
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return Konfiguration{}, fmt.Errorf("konfiguration %s nicht lesbar: %w", pfad, err)
	}

	var k Konfiguration
	if err := yaml.Unmarshal(roh, &k); err != nil {
		return Konfiguration{}, fmt.Errorf("konfiguration %s nicht lesbar: %w", pfad, err)
	}

	if k.Datenbank == "" {
		return Konfiguration{}, fmt.Errorf("konfiguration %s: feld \"datenbank\" fehlt", pfad)
	}
	if k.Adresse == "" {
		k.Adresse = ":8080"
	}
	if k.SaalDatei == "" {
		k.SaalDatei = "saal.json"
	}
	if k.SitzungsDatei == "" {
		k.SitzungsDatei = "sitzung.json"
	}
	if k.MaxOffeneMikrofone < 1 && k.EinmessungReserveDB == 0 {
		return Konfiguration{}, fmt.Errorf("konfiguration %s: max_offene_mikrofone muss mindestens 1 sein — oder einmessung_reserve_db setzen, dann folgt die zahl aus der messung", pfad)
	}
	if k.EinmessungReserveDB != 0 {
		ausMessung, err := ton.MaxOffeneMikrofone(k.EinmessungReserveDB)
		if err != nil {
			return Konfiguration{}, fmt.Errorf("konfiguration %s: %w", pfad, err)
		}
		switch {
		case k.MaxOffeneMikrofone == 0:
			k.MaxOffeneMikrofone = ausMessung
		case k.MaxOffeneMikrofone > ausMessung:
			// Die Messung ist die Wahrheit. Eine Konfiguration darüber wäre
			// die Rückkopplung mit Ansage — abweisen statt leiser stellen.
			return Konfiguration{}, fmt.Errorf(
				"konfiguration %s: max_offene_mikrofone %d, aber die reserve von %.1f dB trägt nur %d — jede verdopplung offener mikrofone kostet 3 dB",
				pfad, k.MaxOffeneMikrofone, k.EinmessungReserveDB, ausMessung)
		}
	}
	if k.KameraZeitlimitMs < 1 {
		k.KameraZeitlimitMs = 500
	}
	if k.SiegelSchluessel == "" {
		k.SiegelSchluessel = "schluessel/kette.key"
	}
	return k, nil
}

// KameraZeitlimit ist das Zeitlimit als Dauer.
func (k Konfiguration) KameraZeitlimit() time.Duration {
	return time.Duration(k.KameraZeitlimitMs) * time.Millisecond
}
