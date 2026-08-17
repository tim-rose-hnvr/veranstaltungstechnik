package kern

import "fmt"

// Stufe ist die Vertraulichkeit einer Unterlage. Die Leiter ist absichtlich
// kurz: vier Stufen, die sich tatsächlich unterscheiden. Wer eine fünfte
// braucht, hat meist ein Rollenproblem, kein Stufenproblem.
type Stufe string

const (
	// StufeOeffentlich: jeder im Saal, auch Gäste.
	StufeOeffentlich Stufe = "oeffentlich"
	// StufeIntern: alle Teilnehmenden, keine Gäste.
	StufeIntern Stufe = "intern"
	// StufeVertraulich: nur Stimmberechtigte. Die Schriftführung hört die
	// Beratung, bekommt das Papier aber nicht — das ist bei ausgelagerter
	// Protokollführung der Normalfall.
	StufeVertraulich Stufe = "vertraulich"
	// StufeGeheim: nur die zur Sitzungsleitung Berechtigten.
	StufeGeheim Stufe = "geheim"
)

// StufeLesen prüft eine Stufe aus Datei oder Datenbank.
func StufeLesen(text string) (Stufe, error) {
	switch s := Stufe(text); s {
	case StufeOeffentlich, StufeIntern, StufeVertraulich, StufeGeheim:
		return s, nil
	default:
		return "", fmt.Errorf("unbekannte vertraulichkeitsstufe %q", text)
	}
}

// DarfSehen beantwortet die Stufenfrage für eine Rolle. Sie ist die einzige
// Stelle, an der darüber entschieden wird.
func (s Stufe) DarfSehen(rolle Rolle) bool {
	switch s {
	case StufeOeffentlich:
		return rolle == RolleLeitung || rolle == RolleDelegierter ||
			rolle == RolleSchriftfuehrung || rolle == RolleGast
	case StufeIntern:
		return rolle == RolleLeitung || rolle == RolleDelegierter || rolle == RolleSchriftfuehrung
	case StufeVertraulich:
		return rolle.Stimmrecht()
	case StufeGeheim:
		return rolle == RolleLeitung
	default:
		// Eine unbekannte Stufe ist die strengste. Ein Tippfehler in der
		// Sitzungsdatei darf keine Unterlage öffnen, die verschlossen bleiben
		// sollte.
		return false
	}
}

// Unterlage ist ein Dokument der Sitzungsmappe. Der Inhalt liegt auf der
// Platte; hier steht, wo, wie vertraulich und zu welchem Punkt.
//
// Pruefsumme ist der SHA-256 der Datei beim Einlesen. Weicht sie beim Abruf
// ab, wurde die Datei unter dem laufenden System ausgetauscht — dann wird
// nicht ausgeliefert.
type Unterlage struct {
	ID         string
	Nummer     int // Reihenfolge innerhalb des Punktes
	TopNummer  int // 0: gehört zur Sitzung, nicht zu einem Punkt
	Titel      string
	Datei      string // Pfad auf der Platte
	Dateiname  string // wie sie beim Abruf heißt
	Typ        string // MIME-Typ
	Groesse    int64
	Stufe      Stufe
	Pruefsumme string
}

// UnterlageZustand ist eine Unterlage, wie sie ein bestimmter Platz sieht.
// Was er nicht sehen darf, taucht hier nicht auf — nicht ausgegraut, sondern
// gar nicht.
type UnterlageZustand struct {
	ID        string `json:"id"`
	TopNummer int    `json:"top"`
	Titel     string `json:"titel"`
	Dateiname string `json:"dateiname"`
	Typ       string `json:"typ"`
	Groesse   int64  `json:"groesse"`
	Stufe     Stufe  `json:"stufe"`
}

// Freigabe ist die Erlaubnis, eine Unterlage einmal abzurufen. Sie gilt kurz
// und nur für den Platz, der sie bekommen hat.
//
// Der Umweg über eine Freigabe hat einen Grund: HTTP kennt den Platz nicht,
// der WebSocket schon. Die Rechtefrage wird dort entschieden, wo die Rechte
// liegen — im Kern —, und die Auslieferung bekommt nur noch eine Marke.
type Freigabe struct {
	Marke         string `json:"marke"`
	Unterlage     string `json:"unterlage"`
	Titel         string `json:"titel"`
	Dateiname     string `json:"dateiname"`
	Typ           string `json:"typ"`
	Wasserzeichen string `json:"wasserzeichen"`
}
