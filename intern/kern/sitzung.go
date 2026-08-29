package kern

import "fmt"

// Sitzungszustand ist die Zustandskette einer Sitzung.
// vorbereitet → bereit → laufend ⇄ unterbrochen → geschlossen → archiviert
type Sitzungszustand string

const (
	SitzungVorbereitet  Sitzungszustand = "vorbereitet"
	SitzungBereit       Sitzungszustand = "bereit"
	SitzungLaufend      Sitzungszustand = "laufend"
	SitzungUnterbrochen Sitzungszustand = "unterbrochen"
	SitzungGeschlossen  Sitzungszustand = "geschlossen"
	SitzungArchiviert   Sitzungszustand = "archiviert"
)

// Teilnahmezustand ist die Zustandskette einer Teilnahme.
type Teilnahmezustand string

const (
	TeilnahmeEingeladen  Teilnahmezustand = "eingeladen"
	TeilnahmeRegistriert Teilnahmezustand = "registriert"
	TeilnahmeAngemeldet  Teilnahmezustand = "angemeldet"
	TeilnahmeAnwesend    Teilnahmezustand = "anwesend"
	TeilnahmeAbwesend    Teilnahmezustand = "abwesend"
)

// Wortzustand ist die Zustandskette einer Wortmeldung.
type Wortzustand string

const (
	WortGemeldet       Wortzustand = "gemeldet"
	WortErteilt        Wortzustand = "erteilt"
	WortLaufend        Wortzustand = "laufend"
	WortBeendet        Wortzustand = "beendet"
	WortEntzogen       Wortzustand = "entzogen"
	WortZurueckgezogen Wortzustand = "zurueckgezogen"
)

// wortUebergaenge hält fest, was auf was folgen darf. Was hier nicht steht,
// wird abgewiesen — eine beendete Wortmeldung wird nie wieder laufend.
var wortUebergaenge = map[Wortzustand][]Wortzustand{
	WortGemeldet:       {WortErteilt, WortZurueckgezogen, WortEntzogen},
	WortErteilt:        {WortLaufend, WortEntzogen, WortBeendet},
	WortLaufend:        {WortBeendet, WortEntzogen},
	WortBeendet:        {},
	WortEntzogen:       {},
	WortZurueckgezogen: {},
}

// WortUebergangErlaubt prüft einen Übergang der Wortmeldung.
func WortUebergangErlaubt(von, nach Wortzustand) bool {
	for _, moeglich := range wortUebergaenge[von] {
		if moeglich == nach {
			return true
		}
	}
	return false
}

// WortzustandLesen prüft einen Zustand aus der Datenbank.
func WortzustandLesen(text string) (Wortzustand, error) {
	switch z := Wortzustand(text); z {
	case WortGemeldet, WortErteilt, WortLaufend, WortBeendet, WortEntzogen, WortZurueckgezogen:
		return z, nil
	default:
		return "", fmt.Errorf("unbekannter wortzustand %q", text)
	}
}

// Offen sagt, ob eine Wortmeldung noch in der Redeliste steht.
func (z Wortzustand) Offen() bool {
	return z == WortGemeldet || z == WortErteilt || z == WortLaufend
}

// Teilnahmeaufbau verbindet Person, Platz und Rolle für eine Sitzung.
// Er kommt aus der Datenbank und ändert sich während der Sitzung nicht.
type Teilnahmeaufbau struct {
	ID          string
	PlatzNummer int
	Person      string
	Rolle       Rolle
	PinHash     []byte
	// Widerspruch gegen die Aufzeichnung. Die Kameranachführung überspringt
	// diesen Platz — das Mikrofon geht trotzdem auf, denn Reden und Gefilmtwerden
	// sind zweierlei.
	Widerspruch bool
}

// Wortmeldung ist ein Eintrag der Redeliste.
type Wortmeldung struct {
	ID          string
	FolgeNr     int64
	PlatzNummer int
	Zustand     Wortzustand
}

// SitzungZustand ist die Sitzung, wie sie der Client sieht.
type SitzungZustand struct {
	Saal         string          `json:"saal"`
	Titel        string          `json:"titel"`
	Zustand      Sitzungszustand `json:"zustand"`
	LeitungPlatz int             `json:"leitung_platz"`
	AktuellerTop int             `json:"aktueller_top"`
}

// RedelisteEintrag ist eine Zeile der Redeliste für den Client.
type RedelisteEintrag struct {
	Platz   int         `json:"platz"`
	Person  string      `json:"person"`
	Zustand Wortzustand `json:"zustand"`
}

// IchZustand ist der einzige Teil des Zustands, der je Verbindung verschieden
// ist: wer ich bin und was ich gerade darf.
type IchZustand struct {
	Platz  int      `json:"platz"`
	Person string   `json:"person"`
	Rolle  Rolle    `json:"rolle"`
	Darf   []string `json:"darf"`
	// Unterlagen ist die Sitzungsmappe aus der Sicht dieses Platzes. Was die
	// Rolle nicht sehen darf, steht hier nicht — auch nicht ausgegraut.
	Unterlagen []UnterlageZustand `json:"unterlagen"`
}
