// Paket web bedient die Oberfläche und den WebSocket-Kanal.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/kern"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/protokoll"
	"github.com/tim-rose-hnvr/veranstaltungstechnik/intern/vorabcheck"
)

// Nachricht ist das, was der Client schickt. Das Feld typ entscheidet.
// platz ist bei mikro_an, wort_erteilen und Verwandten der Zielplatz.
type Nachricht struct {
	Typ   string `json:"typ"`
	Platz int    `json:"platz"`
	Pin   string `json:"pin"`
	Titel string `json:"titel"`
	Art   string `json:"art"`
	Wahl  string `json:"wahl"`
	Top   int    `json:"top"`
	// Unterlage ist die Kennung eines Dokuments der Sitzungsmappe.
	Unterlage string `json:"unterlage"`
}

// Fehlernachricht geht an genau den Client, dessen Befehl scheiterte.
type Fehlernachricht struct {
	Typ  string `json:"typ"`
	Code string `json:"code"`
	Text string `json:"text"`
}

// zustandNachricht ist der gemeinsame Zustand plus dem verbindungseigenen
// Teil. "ich" ist die einzige Stelle, an der nicht alle dasselbe bekommen.
type zustandNachricht struct {
	kern.Zustand
	Ich *kern.IchZustand `json:"ich"`
}

// Server verbindet den Kern mit HTTP.
type Server struct {
	kern        *kern.Kern
	verzeichnis string
	protokoll   *slog.Logger
	pruefer     *vorabcheck.Pruefer
	schreiber   *protokoll.Schreiber
	sitzungID   string
	emulator    *Emulatordaten

	mu           sync.Mutex
	verbindungen map[*verbindung]struct{}
}

type verbindung struct {
	sitz   *websocket.Conn
	sende  chan []byte
	platz  int // 0 = kein Platz angemeldet
	server *Server
}

// Neu baut den HTTP-Server. verzeichnis ist der Ordner web/ mit den Seiten.
func Neu(k *kern.Kern, verzeichnis string, protokoll *slog.Logger) *Server {
	if protokoll == nil {
		protokoll = slog.Default()
	}
	s := &Server{
		kern:         k,
		verzeichnis:  verzeichnis,
		protokoll:    protokoll,
		verbindungen: make(map[*verbindung]struct{}),
	}
	k.SetzeMelder(s.Verteilen)
	return s
}

// Handler liefert die Routen. Jede Rolle ist eine eigene Seite am selben
// Zustand — Namensschild und Dolmetscherplatz lesen nur mit, sie melden
// keinen Platz an und belegen ihn deshalb nicht.
func (s *Server) Handler() http.Handler {
	weiche := http.NewServeMux()
	weiche.HandleFunc("GET /ws", s.ws)
	weiche.Handle("GET /{$}", s.seite("index.html"))
	weiche.Handle("GET /namensschild", s.seite("namensschild.html"))
	weiche.Handle("GET /dolmetscher", s.seite("dolmetscher.html"))
	weiche.Handle("GET /testumgebung", s.seite("testumgebung.html"))
	weiche.Handle("GET /vorabcheck", s.seite("vorabcheck.html"))
	weiche.HandleFunc("POST /vorabcheck", s.vorabcheck)
	weiche.HandleFunc("GET /protokoll.md", s.protokollSeite)
	weiche.HandleFunc("GET /unterlage/{marke}", s.unterlage)
	// Die Prüfstelle gibt die PINs preis. Ohne Freischaltung in der
	// Konfiguration gibt es ihre Adressen nicht.
	if s.emulator != nil {
		s.emulatorRouten(weiche)
	}
	return weiche
}

// SetzeProtokoll hinterlegt den Protokollschreiber.
func (s *Server) SetzeProtokoll(schreiber *protokoll.Schreiber, sitzungID string) {
	s.schreiber, s.sitzungID = schreiber, sitzungID
}

// protokollSeite liefert das Sitzungsprotokoll als Markdown — lesbar im
// Browser, weiterverarbeitbar überall.
func (s *Server) protokollSeite(w http.ResponseWriter, r *http.Request) {
	if s.schreiber == nil {
		http.Error(w, "Das Protokoll ist nicht eingerichtet.", http.StatusServiceUnavailable)
		return
	}

	zustand := s.kern.Zustand()
	inhalt, err := s.schreiber.Markdown(r.Context(), s.sitzungID, zustand.Sitzung.Titel)
	if err != nil {
		s.protokoll.Error("protokoll nicht erzeugt", "grund", err)
		http.Error(w, "Das Protokoll ließ sich nicht erzeugen.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `inline; filename="protokoll.md"`)
	if _, err := w.Write([]byte(inhalt)); err != nil {
		s.protokoll.Warn("protokoll nicht vollständig gesendet", "grund", err)
	}
}

// SetzeVorabcheck hinterlegt den Prüfer. Ohne ihn antwortet die Seite, dass
// der Vorabcheck nicht eingerichtet ist.
func (s *Server) SetzeVorabcheck(p *vorabcheck.Pruefer) { s.pruefer = p }

// vorabcheck führt den Selbsttest aus und liefert den Bericht als JSON.
// Er fährt dabei die Kameras an — deshalb ein POST und nicht ein GET.
func (s *Server) vorabcheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if s.pruefer == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"fehler": "Der Vorabcheck ist nicht eingerichtet.",
		})
		return
	}

	bericht, err := s.pruefer.Laufen(r.Context())
	if errors.Is(err, vorabcheck.ErrSitzungLaeuft) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"fehler": "Die Sitzung läuft. Der Vorabcheck fährt Kameras und ist deshalb gesperrt.",
		})
		return
	}
	if err != nil {
		s.protokoll.Error("vorabcheck fehlgeschlagen", "grund", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"fehler": err.Error()}) //nolint:errcheck
		return
	}

	s.protokoll.Info("vorabcheck gelaufen",
		"ok", bericht.Ok, "hinweise", bericht.Hinweise, "fehler", bericht.Fehler)
	if err := json.NewEncoder(w).Encode(bericht); err != nil {
		s.protokoll.Error("bericht nicht verpackbar", "grund", err)
	}
}

// seite liefert eine Oberfläche. Sie wird bei jedem Abruf von der Platte
// gelesen — Änderungen wirken ohne Neustart.
func (s *Server) seite(datei string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filepath.Join(s.verzeichnis, datei))
	})
}

// Verteilen schickt den vollständigen Zustand an alle Clients.
func (s *Server) Verteilen() {
	z := s.kern.Zustand()

	s.mu.Lock()
	verbindungen := make([]*verbindung, 0, len(s.verbindungen))
	for v := range s.verbindungen {
		verbindungen = append(verbindungen, v)
	}
	s.mu.Unlock()

	for _, v := range verbindungen {
		v.stelle(z)
	}
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	sitz, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.protokoll.Warn("websocket abgelehnt", "grund", err)
		return
	}

	v := &verbindung{sitz: sitz, sende: make(chan []byte, 8), server: s}
	s.mu.Lock()
	s.verbindungen[v] = struct{}{}
	s.mu.Unlock()

	ctx, beenden := context.WithCancel(r.Context())
	defer beenden()

	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		v.schreiben(ctx)
	}()

	// Neuer Client bekommt sofort den vollständigen Zustand.
	v.stelle(s.kern.Zustand())
	v.lesen(ctx)

	beenden()
	<-fertig
	s.abmelden(v)
	sitz.CloseNow() //nolint:errcheck // Verbindung ist ohnehin am Ende
}

func (s *Server) abmelden(v *verbindung) {
	s.mu.Lock()
	delete(s.verbindungen, v)
	platz := v.platz
	s.mu.Unlock()

	if platz != 0 {
		// Ohne Cancel: das Gerät ist weg, der Platz muss trotzdem frei werden.
		ctx, abbrechen := context.WithTimeout(context.Background(), 5*time.Second)
		defer abbrechen()
		if err := s.kern.Abmelden(ctx, platz); err != nil {
			s.protokoll.Warn("platz nicht freigegeben", "platz", platz, "grund", err)
		}
	}
}

func (v *verbindung) lesen(ctx context.Context) {
	for {
		typ, roh, err := v.sitz.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}

		var n Nachricht
		if err := json.Unmarshal(roh, &n); err != nil {
			v.melde(&kern.Fehler{Code: kern.CodePlatzUnbekannt, Text: "Nachricht nicht lesbar"})
			continue
		}
		v.ausfuehren(ctx, n)
	}
}

// ausfuehren reicht den Befehl an den Kern weiter. Hier wird nichts geprüft —
// jede Rechtefrage beantwortet der Kern.
func (v *verbindung) ausfuehren(ctx context.Context, n Nachricht) {
	s := v.server
	absender := v.eigenerPlatz()
	var err error

	switch n.Typ {
	case "anmelden":
		v.anmelden(ctx, n)
		return
	case "unterlage_abrufen":
		v.unterlageAbrufen(ctx, n)
		return
	case "mikro_an":
		err = s.kern.MikroAn(ctx, absender, n.Platz)
	case "mikro_aus":
		err = s.kern.MikroAus(ctx, absender, n.Platz)
	case kern.AktionWortMelden:
		err = s.kern.WortMelden(ctx, absender)
	case kern.AktionWortZurueckziehen:
		err = s.kern.WortZurueckziehen(ctx, absender)
	case kern.AktionWortErteilen:
		err = s.kern.WortErteilen(ctx, absender, n.Platz)
	case kern.AktionWortEntziehen:
		err = s.kern.WortEntziehen(ctx, absender, n.Platz)
	case kern.AktionLeitungUebergeben:
		err = s.kern.LeitungUebergeben(ctx, absender, n.Platz)
	case kern.AktionLeitungUebernehmen:
		err = s.kern.LeitungUebernehmen(ctx, absender)
	case kern.AktionSitzungEroeffnen:
		err = s.kern.SitzungEroeffnen(ctx, absender)
	case kern.AktionSitzungSchliessen:
		err = s.kern.SitzungSchliessen(ctx, absender)
	case kern.AktionTopAufrufen:
		err = s.kern.TopAufrufen(ctx, absender, n.Top)
	case kern.AktionTopAbschliessen:
		err = s.kern.TopAbschliessen(ctx, absender)
	case kern.AktionTopVertagen:
		err = s.kern.TopVertagen(ctx, absender)
	case kern.AktionAbstimmungStarten:
		art, fehler := kern.ArtLesen(n.Art)
		if fehler != nil {
			v.melde(&kern.Fehler{Code: kern.CodeKeineAbstimmung, Text: fehler.Error()})
			return
		}
		err = s.kern.AbstimmungStarten(ctx, absender, n.Titel, art)
	case kern.AktionStimmeAbgeben:
		wahl, fehler := kern.WahlLesen(n.Wahl)
		if fehler != nil {
			v.melde(&kern.Fehler{Code: kern.CodeKeineAbstimmung, Text: fehler.Error()})
			return
		}
		err = s.kern.StimmeAbgeben(ctx, absender, wahl)
	case kern.AktionAbstimmungBeenden:
		err = s.kern.AbstimmungBeenden(ctx, absender)
	case kern.AktionAbstimmungFeststellen:
		err = s.kern.AbstimmungFeststellen(ctx, absender)
	case kern.AktionAbstimmungAbbrechen:
		err = s.kern.AbstimmungAbbrechen(ctx, absender)
	default:
		return
	}
	v.antworten(n, err)
}

// unterlageAbrufen holt eine Freigabe und schickt sie an genau diesen Client.
// Die Rechtefrage beantwortet der Kern; hier wird nur weitergereicht.
func (v *verbindung) unterlageAbrufen(ctx context.Context, n Nachricht) {
	freigabe, err := v.server.kern.UnterlageAbrufen(ctx, v.eigenerPlatz(), n.Unterlage)
	if err != nil {
		v.antworten(n, err)
		return
	}
	roh, err := json.Marshal(struct {
		Typ string `json:"typ"`
		*kern.Freigabe
	}{Typ: "unterlage", Freigabe: freigabe})
	if err != nil {
		v.server.protokoll.Error("freigabe nicht verpackbar", "grund", err)
		return
	}
	v.abschicken(roh)
}

// unterlage liefert das Dokument gegen eine Marke aus. Die Marke gilt einmal
// und kurz; wer sie weitergibt, gibt nichts Brauchbares weiter.
func (s *Server) unterlage(w http.ResponseWriter, r *http.Request) {
	inhalt, unterlage, wasserzeichen, err := s.kern.UnterlageOeffnen(r.PathValue("marke"))
	if err != nil {
		var f *kern.Fehler
		if errors.As(err, &f) {
			http.Error(w, f.Text, http.StatusForbidden)
			return
		}
		http.Error(w, "Die Unterlage ist nicht verfügbar.", http.StatusInternalServerError)
		return
	}
	defer inhalt.Close()

	w.Header().Set("Content-Type", unterlage.Typ)
	w.Header().Set("Content-Length", strconv.FormatInt(unterlage.Groesse, 10))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Content-Disposition", "inline; filename=\""+unterlage.Dateiname+"\"")
	// Das Wasserzeichen steht auch im Kopf: wer die Datei speichert und später
	// vorlegt, hat den Beleg mitgespeichert.
	w.Header().Set("X-Wasserzeichen", wasserzeichen)
	w.Header().Set("X-Vertraulichkeit", string(unterlage.Stufe))

	if _, err := io.Copy(w, inhalt); err != nil {
		s.protokoll.Warn("unterlage nicht vollständig gesendet",
			"unterlage", unterlage.ID, "grund", err)
	}
}

func (v *verbindung) anmelden(ctx context.Context, n Nachricht) {
	s := v.server
	vorher := v.eigenerPlatz()
	if vorher == n.Platz {
		return
	}

	if err := s.kern.Anmelden(ctx, n.Platz, n.Pin); err != nil {
		v.antworten(n, err)
		return
	}
	if vorher != 0 {
		if err := s.kern.Abmelden(ctx, vorher); err != nil {
			s.protokoll.Warn("alter platz nicht freigegeben", "platz", vorher, "grund", err)
		}
	}
	s.mu.Lock()
	v.platz = n.Platz
	s.mu.Unlock()

	// Die Rechte hängen am Platz — nach der Anmeldung neu verteilen.
	s.Verteilen()
}

func (v *verbindung) antworten(n Nachricht, err error) {
	if err == nil {
		return
	}
	var f *kern.Fehler
	if errors.As(err, &f) {
		v.melde(f)
		return
	}
	v.server.protokoll.Error("befehl fehlgeschlagen", "typ", n.Typ, "platz", n.Platz, "grund", err)
}

func (v *verbindung) eigenerPlatz() int {
	v.server.mu.Lock()
	defer v.server.mu.Unlock()
	return v.platz
}

func (v *verbindung) melde(f *kern.Fehler) {
	roh, err := json.Marshal(Fehlernachricht{Typ: "fehler", Code: f.Code, Text: f.Text})
	if err != nil {
		return
	}
	v.abschicken(roh)
}

func (v *verbindung) stelle(z kern.Zustand) {
	n := zustandNachricht{Zustand: z}
	if platz := v.eigenerPlatz(); platz != 0 {
		n.Ich = v.server.kern.Ich(platz)
	}
	roh, err := json.Marshal(n)
	if err != nil {
		v.server.protokoll.Error("zustand nicht verpackbar", "grund", err)
		return
	}
	v.abschicken(roh)
}

func (v *verbindung) abschicken(roh []byte) {
	select {
	case v.sende <- roh:
	default:
		// Client kommt nicht nach. Er bekommt beim nächsten Zustand wieder
		// alles — Teiländerungen gibt es nicht, also fehlt nichts.
		v.server.protokoll.Warn("nachricht verworfen, client zu langsam")
	}
}

func (v *verbindung) schreiben(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case roh := <-v.sende:
			frist, abbrechen := context.WithTimeout(ctx, 5*time.Second)
			err := v.sitz.Write(frist, websocket.MessageText, roh)
			abbrechen()
			if err != nil {
				return
			}
		}
	}
}
