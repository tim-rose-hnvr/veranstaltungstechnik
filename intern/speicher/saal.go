// Paket speicher liest die Saaldatei und hält Saal und Ereigniskette in
// PostgreSQL.
package speicher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Saaldaten ist der Inhalt von saal.json.
type Saaldaten struct {
	Saal    string        `json:"saal"`
	Kameras []Kameradaten `json:"kameras"`
	Plaetze []Platzdaten  `json:"plaetze"`
}

// Kameradaten beschreibt eine PTZ-Kamera im Saal.
type Kameradaten struct {
	Name    string `json:"name"`
	Adresse string `json:"adresse"`
	Kanal   uint8  `json:"kanal"`
}

// Platzdaten beschreibt einen Sitzplatz und seinen Preset.
type Platzdaten struct {
	Nummer int    `json:"nummer"`
	Name   string `json:"name"`
	Kamera string `json:"kamera"`
	Preset uint8  `json:"preset"`
}

// SaalLesen liest die Saaldatei und prüft sie auf Vollständigkeit.
func SaalLesen(pfad string) (Saaldaten, error) {
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return Saaldaten{}, fmt.Errorf("saaldatei %s nicht lesbar: %w", pfad, err)
	}

	var d Saaldaten
	entschluessler := json.NewDecoder(bytes.NewReader(roh))
	entschluessler.DisallowUnknownFields()
	if err := entschluessler.Decode(&d); err != nil {
		return Saaldaten{}, fmt.Errorf("saaldatei %s nicht lesbar: %w", pfad, err)
	}
	if err := d.Pruefen(); err != nil {
		return Saaldaten{}, fmt.Errorf("saaldatei %s: %w", pfad, err)
	}
	return d, nil
}

// Pruefen stellt sicher, dass die Saaldatei in sich stimmig ist.
func (d Saaldaten) Pruefen() error {
	if d.Saal == "" {
		return fmt.Errorf("feld \"saal\" fehlt")
	}
	if len(d.Plaetze) == 0 {
		return fmt.Errorf("kein platz angegeben")
	}

	kameras := make(map[string]bool, len(d.Kameras))
	for _, k := range d.Kameras {
		if k.Name == "" || k.Adresse == "" {
			return fmt.Errorf("kamera ohne name oder adresse")
		}
		if kameras[k.Name] {
			return fmt.Errorf("kamera %q kommt doppelt vor", k.Name)
		}
		if k.Kanal < 1 || k.Kanal > 7 {
			return fmt.Errorf("kamera %q: kanal %d liegt außerhalb von 1..7", k.Name, k.Kanal)
		}
		kameras[k.Name] = true
	}

	nummern := make(map[int]bool, len(d.Plaetze))
	for _, p := range d.Plaetze {
		if p.Nummer < 1 {
			return fmt.Errorf("platz mit nummer %d", p.Nummer)
		}
		if nummern[p.Nummer] {
			return fmt.Errorf("platz %d kommt doppelt vor", p.Nummer)
		}
		nummern[p.Nummer] = true
		if p.Name == "" {
			return fmt.Errorf("platz %d ohne name", p.Nummer)
		}
		if !kameras[p.Kamera] {
			return fmt.Errorf("platz %d verweist auf unbekannte kamera %q", p.Nummer, p.Kamera)
		}
	}
	return nil
}
