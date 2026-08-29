package web

import (
	"crypto/ed25519"
	"encoding/hex"
	"net/http"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/siegel"
)

// Siegelstelle schließt die Ereigniskette ab und prüft die Abschlüsse.
type Siegelstelle struct {
	siegler     *siegel.Siegler
	oeffentlich ed25519.PublicKey
	saalID      string
	kette       Kettenleser
}

// SetzeSiegler hinterlegt den Siegler. Ohne ihn antwortet die Adresse, dass
// nicht gesiegelt wird. Die Kette wird hier eigens hereingereicht: die
// Siegelprüfung gehört in jeden Saal, auch ohne freigeschaltete Prüfstelle.
func (s *Server) SetzeSiegler(siegler *siegel.Siegler, oeffentlich ed25519.PublicKey,
	saalID string, kette Kettenleser) {
	s.siegel = &Siegelstelle{siegler: siegler, oeffentlich: oeffentlich, saalID: saalID, kette: kette}
}

// siegelPruefen liefert den Prüfbericht: geht jedes Siegel auf, und wie viel
// der Kette ist gedeckt. Ein GET, weil nichts geschrieben wird.
func (s *Server) siegelPruefen(w http.ResponseWriter, r *http.Request) {
	if s.siegel == nil || s.siegel.kette == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		s.alsJSON(w, map[string]any{"fehler": "Die Siegelprüfung ist nicht eingerichtet."})
		return
	}

	kette, err := s.siegel.kette.Ereignisse(r.Context(), s.siegel.saalID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.alsJSON(w, map[string]any{"fehler": err.Error()})
		return
	}
	bericht := siegel.Pruefen(s.siegel.saalID, kette, s.siegel.oeffentlich)
	s.alsJSON(w, map[string]any{
		"ok":              bericht.Ok(),
		"siegel":          bericht.Siegel,
		"gedeckt":         bericht.Gedeckt,
		"laenge":          bericht.Laenge,
		"ungedeckt":       bericht.Ungedeckt(),
		"fingerabdruecke": bericht.Fingerabdruecke,
		"fehler":          bericht.Fehler,
		"schluessel":      hex.EncodeToString(s.siegel.oeffentlich),
	})
}

// siegelSetzen schließt die Kette jetzt ab. Ein POST, weil es die Kette
// verlängert.
func (s *Server) siegelSetzen(w http.ResponseWriter, r *http.Request) {
	if s.siegel == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		s.alsJSON(w, map[string]any{"fehler": "Es wird nicht gesiegelt."})
		return
	}
	abschluss, err := s.siegel.siegler.Siegeln(r.Context())
	if err != nil {
		s.protokoll.Error("siegel nicht gesetzt", "grund", err)
		w.WriteHeader(http.StatusInternalServerError)
		s.alsJSON(w, map[string]any{"fehler": err.Error()})
		return
	}
	s.alsJSON(w, abschluss)
}
