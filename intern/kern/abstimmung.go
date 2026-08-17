package kern

import "fmt"

// Abstimmungsart bestimmt, ob die Zuordnung Stimme zu Person existiert.
type Abstimmungsart string

const (
	// AbstimmungOffen: Handzeichen-Ersatz, die Stimme ist der Person zugeordnet.
	AbstimmungOffen Abstimmungsart = "offen"
	// AbstimmungNamentlich: wie offen, das Ergebnis wird je Person ausgewiesen.
	AbstimmungNamentlich Abstimmungsart = "namentlich"
	// AbstimmungGeheim: die Zuordnung Stimme zu Person existiert nirgends.
	AbstimmungGeheim Abstimmungsart = "geheim"
)

// ArtLesen prüft eine Abstimmungsart aus Protokoll oder Datenbank.
func ArtLesen(text string) (Abstimmungsart, error) {
	switch a := Abstimmungsart(text); a {
	case AbstimmungOffen, AbstimmungNamentlich, AbstimmungGeheim:
		return a, nil
	default:
		return "", fmt.Errorf("unbekannte abstimmungsart %q", text)
	}
}

// Geheim sagt, ob die Zuordnung Stimme zu Person unterbleiben muss.
func (a Abstimmungsart) Geheim() bool { return a == AbstimmungGeheim }

// Abstimmungszustand ist die Zustandskette einer Abstimmung.
// vorbereitet → laufend → ausgezaehlt → festgestellt | abgebrochen
type Abstimmungszustand string

const (
	AbstimmungVorbereitet  Abstimmungszustand = "vorbereitet"
	AbstimmungLaufend      Abstimmungszustand = "laufend"
	AbstimmungAusgezaehlt  Abstimmungszustand = "ausgezaehlt"
	AbstimmungFestgestellt Abstimmungszustand = "festgestellt"
	AbstimmungAbgebrochen  Abstimmungszustand = "abgebrochen"
)

// Wahl ist die einzelne Stimme.
type Wahl string

const (
	WahlJa         Wahl = "ja"
	WahlNein       Wahl = "nein"
	WahlEnthaltung Wahl = "enthaltung"
)

// WahlLesen prüft eine Stimme aus Protokoll oder Datenbank.
func WahlLesen(text string) (Wahl, error) {
	switch w := Wahl(text); w {
	case WahlJa, WahlNein, WahlEnthaltung:
		return w, nil
	default:
		return "", fmt.Errorf("unbekannte stimme %q", text)
	}
}

// Abstimmung ist der laufende oder zuletzt abgeschlossene Vorgang.
type Abstimmung struct {
	ID      string
	FolgeNr int64
	Titel   string
	Art     Abstimmungsart
	Zustand Abstimmungszustand

	// Beim Start eingefroren.
	Stimmberechtigt int
	Anwesend        int
	Quorum          int

	// Wer schon abgestimmt hat — nie, wie. Bei geheimer Wahl ist das die
	// einzige Verbindung zur Person, und sie sagt nichts über den Inhalt.
	Abgegeben map[int]bool
	// Nur bei offener und namentlicher Abstimmung gefüllt.
	Namentlich map[int]Wahl
	// Zählung, erst ab dem Auszählen sichtbar.
	Ja, Nein, Enthaltung int
}

// Angenommen gilt bei einfacher Mehrheit: mehr Ja als Nein.
// Enthaltungen zählen nicht mit.
func (a *Abstimmung) Angenommen() bool { return a.Ja > a.Nein }

// Beschlussfaehig sagt, ob genug Stimmberechtigte anwesend waren.
func (a *Abstimmung) Beschlussfaehig() bool { return a.Anwesend >= a.Quorum }

// Laeuft sagt, ob gerade abgestimmt wird. Während dessen sind automatische
// Technikeingriffe gesperrt.
func (a *Abstimmung) Laeuft() bool { return a != nil && a.Zustand == AbstimmungLaufend }

// Quorum ist die einfache Mehrheit der Stimmberechtigten: mehr als die Hälfte.
func Quorum(stimmberechtigt int) int { return stimmberechtigt/2 + 1 }

// Stimmrecht hat, wer leitet oder delegiert ist. Schriftführung und Gäste
// stimmen nicht ab.
func (r Rolle) Stimmrecht() bool { return r == RolleLeitung || r == RolleDelegierter }

// AbstimmungZustand ist die Abstimmung, wie sie der Client sieht.
// Die Zählung bleibt bis zum Auszählen leer — ein Zwischenstand würde die
// noch ausstehenden Stimmen beeinflussen.
type AbstimmungZustand struct {
	Titel           string             `json:"titel"`
	Art             Abstimmungsart     `json:"art"`
	Zustand         Abstimmungszustand `json:"zustand"`
	Stimmberechtigt int                `json:"stimmberechtigt"`
	Anwesend        int                `json:"anwesend"`
	Quorum          int                `json:"quorum"`
	Abgegeben       int                `json:"abgegeben"`
	Ergebnis        *Ergebnis          `json:"ergebnis"`
}

// Ergebnis steht erst nach dem Auszählen.
type Ergebnis struct {
	Ja          int            `json:"ja"`
	Nein        int            `json:"nein"`
	Enthaltung  int            `json:"enthaltung"`
	Angenommen  bool           `json:"angenommen"`
	Namentlich  map[int]Wahl   `json:"namentlich"`
	Abstimmende map[int]bool   `json:"-"`
	Art         Abstimmungsart `json:"-"`
}
