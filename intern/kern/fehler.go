package kern

// Fehlercodes des WebSocket-Protokolls.
const (
	CodeGrenzeErreicht = "grenze_erreicht"
	CodePlatzUnbekannt = "platz_unbekannt"
	CodePlatzBelegt    = "platz_belegt"
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
