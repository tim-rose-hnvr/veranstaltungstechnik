package kern

import "fmt"

// Topzustand ist die Zustandskette eines Tagesordnungspunkts.
// offen → laufend → abgeschlossen | vertagt
type Topzustand string

const (
	TopOffen         Topzustand = "offen"
	TopLaufend       Topzustand = "laufend"
	TopAbgeschlossen Topzustand = "abgeschlossen"
	TopVertagt       Topzustand = "vertagt"
)

// topUebergaenge hält fest, was auf was folgen darf. Ein abgeschlossener Punkt
// wird wieder aufrufbar — Wiederaufnahme kommt vor. Ein vertagter nicht: er
// gehört in die nächste Sitzung, nicht in diese.
var topUebergaenge = map[Topzustand][]Topzustand{
	TopOffen:         {TopLaufend, TopVertagt},
	TopLaufend:       {TopAbgeschlossen, TopVertagt},
	TopAbgeschlossen: {TopLaufend},
	TopVertagt:       {},
}

// TopUebergangErlaubt prüft einen Übergang.
func TopUebergangErlaubt(von, nach Topzustand) bool {
	for _, moeglich := range topUebergaenge[von] {
		if moeglich == nach {
			return true
		}
	}
	return false
}

// TopzustandLesen prüft einen Zustand aus Datei oder Datenbank.
func TopzustandLesen(text string) (Topzustand, error) {
	switch z := Topzustand(text); z {
	case TopOffen, TopLaufend, TopAbgeschlossen, TopVertagt:
		return z, nil
	default:
		return "", fmt.Errorf("unbekannter tagesordnungszustand %q", text)
	}
}

// Tagesordnungspunkt ist ein Punkt der Tagesordnung. Er kommt aus der
// Datenbank; die Reihenfolge steht in Nummer.
//
// Oeffentlich entscheidet über Stream und Aufzeichnung: bei einem nicht
// öffentlichen Punkt pausieren beide, und zwar automatisch aus dem
// Sitzungszustand heraus — nicht durch einen Handgriff, den jemand vergessen
// kann.
type Tagesordnungspunkt struct {
	ID          string
	Nummer      int
	Titel       string
	Oeffentlich bool
	Zustand     Topzustand
}

// TopZustand ist ein Tagesordnungspunkt, wie ihn der Client sieht.
type TopZustand struct {
	Nummer      int        `json:"nummer"`
	Titel       string     `json:"titel"`
	Oeffentlich bool       `json:"oeffentlich"`
	Zustand     Topzustand `json:"zustand"`
}
