package kern

import "fmt"

// Rolle bestimmt, was eine Person in einer Sitzung darf. Nicht der Gerätetyp
// entscheidet, sondern die Rolle — geprüft wird hier im Kern, nie in der
// Oberfläche.
type Rolle string

const (
	RolleLeitung         Rolle = "leitung"
	RolleDelegierter     Rolle = "delegierter"
	RolleSchriftfuehrung Rolle = "schriftfuehrung"
	RolleGast            Rolle = "gast"
)

// RolleLesen prüft eine Rolle aus Datei oder Datenbank.
func RolleLesen(text string) (Rolle, error) {
	switch Rolle(text) {
	case RolleLeitung, RolleDelegierter, RolleSchriftfuehrung, RolleGast:
		return Rolle(text), nil
	default:
		return "", fmt.Errorf("unbekannte rolle %q", text)
	}
}

// Aktionen des Protokolls. Sie sind zugleich die Namen, die im Feld "darf"
// an den Client gehen — dann kann er Knöpfe ausgrauen, ohne selbst zu
// entscheiden, was erlaubt ist.
const (
	AktionWortMelden         = "wort_melden"
	AktionWortZurueckziehen  = "wort_zurueckziehen"
	AktionWortErteilen       = "wort_erteilen"
	AktionWortEntziehen      = "wort_entziehen"
	AktionLeitungUebergeben  = "leitung_uebergeben"
	AktionLeitungUebernehmen = "leitung_uebernehmen"
	AktionSitzungEroeffnen   = "sitzung_eroeffnen"
	AktionSitzungSchliessen  = "sitzung_schliessen"
	AktionMikroAn            = "mikro_an"
	AktionMikroAus           = "mikro_aus"

	AktionAbstimmungStarten     = "abstimmung_starten"
	AktionAbstimmungBeenden     = "abstimmung_beenden"
	AktionAbstimmungFeststellen = "abstimmung_feststellen"
	AktionAbstimmungAbbrechen   = "abstimmung_abbrechen"
	AktionStimmeAbgeben         = "stimme_abgeben"
)

// alleAktionen ist die Reihenfolge, in der "darf" aufgebaut wird.
var alleAktionen = []string{
	AktionSitzungEroeffnen, AktionSitzungSchliessen,
	AktionWortMelden, AktionWortZurueckziehen,
	AktionWortErteilen, AktionWortEntziehen,
	AktionLeitungUebergeben, AktionLeitungUebernehmen,
	AktionMikroAn, AktionMikroAus,
	AktionAbstimmungStarten, AktionAbstimmungBeenden,
	AktionAbstimmungFeststellen, AktionAbstimmungAbbrechen,
	AktionStimmeAbgeben,
}

// rolleDarf beantwortet die Frage der Rolle allein: Ist diese Aktion für
// jemanden mit dieser Rolle überhaupt vorgesehen? Ob sie im aktuellen Zustand
// zulässig ist — laufende Sitzung, erteiltes Wort, aktiver Staffelstab —
// entscheidet der Sitzungszustand zusätzlich.
//
// leitungAktiv trennt die berechtigte von der tatsächlich führenden Leitung:
// Wer den Staffelstab nicht hat, darf nichts davon.
func rolleDarf(rolle Rolle, leitungAktiv bool, aktion string) bool {
	switch aktion {
	case AktionSitzungEroeffnen, AktionSitzungSchliessen,
		AktionWortErteilen, AktionWortEntziehen, AktionLeitungUebergeben,
		AktionAbstimmungStarten, AktionAbstimmungBeenden,
		AktionAbstimmungFeststellen, AktionAbstimmungAbbrechen:
		return rolle == RolleLeitung && leitungAktiv

	case AktionLeitungUebernehmen:
		// Übernehmen darf, wer zur Leitung berechtigt ist — auch ohne
		// Staffelstab. Ob der führende Platz wirklich leer ist, entscheidet
		// der Sitzungszustand.
		return rolle == RolleLeitung

	case AktionStimmeAbgeben:
		// Stimmrecht hat, wer leitet oder delegiert ist — nicht die
		// Schriftführung und kein Gast.
		return rolle.Stimmrecht()

	case AktionWortMelden, AktionWortZurueckziehen:
		return rolle == RolleLeitung || rolle == RolleDelegierter

	case AktionMikroAn, AktionMikroAus:
		return rolle == RolleLeitung || rolle == RolleDelegierter

	default:
		return false
	}
}
