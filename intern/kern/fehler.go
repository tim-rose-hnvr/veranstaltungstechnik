package kern

// Fehlercodes des WebSocket-Protokolls.
const (
	CodeGrenzeErreicht = "grenze_erreicht"
	CodePlatzUnbekannt = "platz_unbekannt"
	CodePlatzBelegt    = "platz_belegt"

	// Aus Meilenstein 2.
	CodeNichtBerechtigt    = "nicht_berechtigt"
	CodePinFalsch          = "pin_falsch"
	CodeKeinWort           = "kein_wort"
	CodeSitzungLaeuftNicht = "sitzung_laeuft_nicht"

	// Aus Meilenstein 3.
	CodeKeineAbstimmung      = "keine_abstimmung"
	CodeAbstimmungLaeuft     = "abstimmung_laeuft"
	CodeNichtBeschlussfaehig = "nicht_beschlussfaehig"
	CodeSchonAbgestimmt      = "schon_abgestimmt"

	// Aus Meilenstein 4.
	CodeKeinTop        = "kein_top"
	CodeTopUnbekannt   = "top_unbekannt"
	CodeTopGeschlossen = "top_geschlossen"

	// CodeSpeicherFehler meldet, dass ein Ereignis nicht in die Kette
	// geschrieben werden konnte. Der Zustand bleibt dann unverändert — ein
	// Zustandswechsel ohne Protokolleintrag darf es nicht geben.
	CodeSpeicherFehler = "speicher_fehler"
)

// Fehler ist ein fachlicher Fehler mit einem Code aus dem Protokoll.
type Fehler struct {
	Code string
	Text string
}

func (f *Fehler) Error() string { return f.Code + ": " + f.Text }

func fehler(code, text string) *Fehler { return &Fehler{Code: code, Text: text} }
