// Paket web bedient die Oberfläche und den WebSocket-Kanal.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/tim-rose-hnvr/kameranachverfolgung/intern/kern"
)

// Nachricht ist das, was der Client schickt. Das Feld typ entscheidet.
type Nachricht struct {
	Typ   string `json:"typ"`
	Platz int    `json:"platz"`
}

// Fehlernachricht geht an genau den Client, dessen Befehl scheiterte.
type Fehlernachricht struct {
	Typ  string `json:"typ"`
	Code string `json:"code"`
	Text string `json:"text"`
}

// Server verbindet den Kern mit HTTP.
type Server struct {
	kern      *kern.Kern
	indexPfad string
	protokoll *slog.Logger

	mu           sync.Mutex
	verbindungen map[*verbindung]struct{}
}

type verbindung struct {
	sitz   *websocket.Conn
	sende  chan []byte
	platz  int // 0 = kein Platz angemeldet
	server *Server
}

// Neu baut den HTTP-Server. indexPfad zeigt auf web/index.html.
func Neu(k *kern.Kern, indexPfad string, protokoll *slog.Logger) *Server {
	if protokoll == nil {
		protokoll = slog.Default()
	}
	s := &Server{
		kern:         k,
		indexPfad:    indexPfad,
		protokoll:    protokoll,
		verbindungen: make(map[*verbindung]struct{}),
	}
	k.SetzeMelder(s.Verteilen)
	return s
}

// Handler liefert die Routen.
func (s *Server) Handler() http.Handler {
	weiche := http.NewServeMux()
	weiche.HandleFunc("GET /ws", s.ws)
	weiche.HandleFunc("GET /{$}", s.index)
	return weiche
}

// index liefert die Oberfläche. Sie wird bei jedem Abruf von der Platte
// gelesen — Änderungen wirken ohne Neustart.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, s.indexPfad)
}

// Verteilen schickt den vollständigen Zustand an alle Clients.
func (s *Server) Verteilen(z kern.Zustand) {
	roh, err := json.Marshal(z)
	if err != nil {
		s.protokoll.Error("zustand nicht verpackbar", "grund", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for v := range s.verbindungen {
		select {
		case v.sende <- roh:
		default:
			// Client kommt nicht nach. Er bekommt beim nächsten Zustand
			// wieder alles — Teiländerungen gibt es nicht, also fehlt nichts.
			s.protokoll.Warn("zustand verworfen, client zu langsam")
		}
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
			v.melde(&kern.Fehler{Code: "platz_unbekannt", Text: "Nachricht nicht lesbar"})
			continue
		}
		v.ausfuehren(ctx, n)
	}
}

func (v *verbindung) ausfuehren(ctx context.Context, n Nachricht) {
	s := v.server
	var err error

	switch n.Typ {
	case "anmelden":
		v.server.mu.Lock()
		vorher := v.platz
		v.server.mu.Unlock()
		if vorher == n.Platz {
			return
		}
		if err = s.kern.Anmelden(ctx, n.Platz); err == nil {
			if vorher != 0 {
				if fehler := s.kern.Abmelden(ctx, vorher); fehler != nil {
					s.protokoll.Warn("alter platz nicht freigegeben", "platz", vorher, "grund", fehler)
				}
			}
			v.server.mu.Lock()
			v.platz = n.Platz
			v.server.mu.Unlock()
		}
	case "mikro_an":
		err = s.kern.MikroAn(ctx, n.Platz)
	case "mikro_aus":
		err = s.kern.MikroAus(ctx, n.Platz)
	default:
		return
	}

	if err != nil {
		var f *kern.Fehler
		if errors.As(err, &f) {
			v.melde(f)
			return
		}
		s.protokoll.Error("befehl fehlgeschlagen", "typ", n.Typ, "platz", n.Platz, "grund", err)
	}
}

func (v *verbindung) melde(f *kern.Fehler) {
	roh, err := json.Marshal(Fehlernachricht{Typ: "fehler", Code: f.Code, Text: f.Text})
	if err != nil {
		return
	}
	select {
	case v.sende <- roh:
	default:
	}
}

func (v *verbindung) stelle(z kern.Zustand) {
	roh, err := json.Marshal(z)
	if err != nil {
		return
	}
	select {
	case v.sende <- roh:
	default:
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
