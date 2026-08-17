package kamera

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// Attrappe ist eine PTZ-Kamera, die es nicht gibt: sie hört auf demselben
// UDP-Port, versteht dasselbe VISCA over IP und quittiert wie das Gerät im
// Rack. Damit läuft der ganze Weg — Kern, Steuerung, Socket, Rahmen — auch
// ohne Kamera im Raum, und was ankommt, lässt sich Byte für Byte ansehen.
//
// Sie ist nicht dazu da, den Betrieb zu ersetzen, sondern ihn vorführbar und
// prüfbar zu machen. Im Saal steht eine echte Kamera.
type Attrappe struct {
	protokoll *slog.Logger

	mu       sync.Mutex
	geraete  []*attrappenKamera
	empfang  []Empfang
	schranke int // wie viele Empfänge aufgehoben werden
}

type attrappenKamera struct {
	name   string
	sockel *net.UDPConn
}

// Empfang ist ein Befehl, wie ihn die Kamera gesehen hat.
type Empfang struct {
	Kamera  string    `json:"kamera"`
	Adresse string    `json:"adresse"`
	Von     string    `json:"von"`
	Zeit    time.Time `json:"zeit"`
	Folge   uint32    `json:"folge"`
	Kanal   uint8     `json:"kanal"`
	Preset  uint8     `json:"preset"`
	Roh     string    `json:"roh"`     // der Rahmen als Hex, so wie er über das Netz kam
	Deutung string    `json:"deutung"` // was der Rahmen bedeutet, im Klartext
}

// Stand ist die Lage einer simulierten Kamera.
type Stand struct {
	Name    string `json:"name"`
	Adresse string `json:"adresse"`
	Preset  uint8  `json:"preset"`  // 0: noch nie gefahren
	Befehle int    `json:"befehle"` // wie viele Befehle sie bekommen hat
}

// NeuAttrappe erzeugt die Simulation. Sie hört noch auf nichts — dafür ist
// Anschalten da.
func NeuAttrappe(protokoll *slog.Logger) *Attrappe {
	if protokoll == nil {
		protokoll = slog.Default()
	}
	return &Attrappe{protokoll: protokoll, schranke: 200}
}

// Anschalten lässt eine simulierte Kamera auf einer Adresse hören und liefert
// die tatsächlich belegte Adresse zurück — bei Port 0 ist das eine andere als
// die angefragte.
func (a *Attrappe) Anschalten(name, adresse string) (string, error) {
	if _, _, err := net.SplitHostPort(adresse); err != nil {
		adresse = net.JoinHostPort(adresse, fmt.Sprint(Standardport))
	}
	ziel, err := net.ResolveUDPAddr("udp", adresse)
	if err != nil {
		return "", fmt.Errorf("kamera %q: adresse %q: %w", name, adresse, err)
	}
	sockel, err := net.ListenUDP("udp", ziel)
	if err != nil {
		return "", fmt.Errorf("kamera %q auf %s: %w", name, adresse, err)
	}

	g := &attrappenKamera{name: name, sockel: sockel}
	a.mu.Lock()
	a.geraete = append(a.geraete, g)
	a.mu.Unlock()

	go a.hoeren(g)
	return sockel.LocalAddr().String(), nil
}

// Schliessen schaltet alle simulierten Kameras ab.
func (a *Attrappe) Schliessen() {
	a.mu.Lock()
	geraete := a.geraete
	a.geraete = nil
	a.mu.Unlock()

	for _, g := range geraete {
		g.sockel.Close() //nolint:errcheck // beim Herunterfahren ohne Belang
	}
}

// Empfangen liefert die Befehle, neueste zuletzt.
func (a *Attrappe) Empfangen() []Empfang {
	a.mu.Lock()
	defer a.mu.Unlock()
	liste := make([]Empfang, len(a.empfang))
	copy(liste, a.empfang)
	return liste
}

// Staende liefert die Lage jeder simulierten Kamera.
func (a *Attrappe) Staende() []Stand {
	a.mu.Lock()
	defer a.mu.Unlock()

	nach := make(map[string]*Stand, len(a.geraete))
	reihe := make([]*Stand, 0, len(a.geraete))
	for _, g := range a.geraete {
		s := &Stand{Name: g.name, Adresse: g.sockel.LocalAddr().String()}
		nach[g.name] = s
		reihe = append(reihe, s)
	}
	for _, e := range a.empfang {
		if s, bekannt := nach[e.Kamera]; bekannt {
			s.Preset = e.Preset
			s.Befehle++
		}
	}

	staende := make([]Stand, 0, len(reihe))
	for _, s := range reihe {
		staende = append(staende, *s)
	}
	return staende
}

func (a *Attrappe) hoeren(g *attrappenKamera) {
	puffer := make([]byte, 64)
	for {
		anzahl, absender, err := g.sockel.ReadFromUDP(puffer)
		if err != nil {
			return // Sockel geschlossen
		}
		rahmen := puffer[:anzahl]

		e := Empfang{
			Kamera:  g.name,
			Adresse: g.sockel.LocalAddr().String(),
			Von:     absender.String(),
			Zeit:    time.Now(),
			Roh:     hexMitAbstand(rahmen),
		}
		kanal, preset, folge, err := RahmenLesen(rahmen)
		if err != nil {
			e.Deutung = "nicht verstanden: " + err.Error()
			a.protokoll.Warn("attrappe: rahmen nicht verstanden",
				"kamera", g.name, "grund", err, "roh", e.Roh)
		} else {
			e.Kanal, e.Preset, e.Folge = kanal, preset, folge
			e.Deutung = fmt.Sprintf("Preset %d abrufen, Kanal %d, Folge %d", preset, kanal, folge)
			// Eine echte Kamera quittiert erst, dann meldet sie die Ausführung.
			// Ohne Antwort gilt sie dem Kern als nicht erreichbar.
			a.antworten(g, absender, folge)
		}

		a.mu.Lock()
		a.empfang = append(a.empfang, e)
		if len(a.empfang) > a.schranke {
			a.empfang = a.empfang[len(a.empfang)-a.schranke:]
		}
		a.mu.Unlock()
	}
}

// antworten schickt Acknowledge und Completion, wie es das Gerät tut.
func (a *Attrappe) antworten(g *attrappenKamera, an *net.UDPAddr, folge uint32) {
	for _, nutzlast := range [][]byte{
		{0x90, 0x41, 0xFF}, // Acknowledge
		{0x90, 0x51, 0xFF}, // Completion
	} {
		antwort := make([]byte, 0, 8+len(nutzlast))
		antwort = append(antwort, 0x01, 0x11) // Nutzlasttyp „VISCA reply"
		antwort = binary.BigEndian.AppendUint16(antwort, uint16(len(nutzlast)))
		antwort = binary.BigEndian.AppendUint32(antwort, folge)
		antwort = append(antwort, nutzlast...)

		if _, err := g.sockel.WriteToUDP(antwort, an); err != nil {
			a.protokoll.Warn("attrappe: antwort nicht gesendet", "kamera", g.name, "grund", err)
			return
		}
	}
}

// RahmenLesen zerlegt einen „Preset abrufen"-Rahmen wieder. Sie ist die
// Gegenrichtung zu Rahmen und prüft dabei jedes Byte, das eine echte Kamera
// auch prüfen würde.
func RahmenLesen(rahmen []byte) (kanal, preset uint8, folge uint32, err error) {
	if len(rahmen) < 8 {
		return 0, 0, 0, fmt.Errorf("rahmen ist %d byte lang, der kopf allein braucht 8", len(rahmen))
	}
	if rahmen[0] != nutzlasttypBefehl[0] || rahmen[1] != nutzlasttypBefehl[1] {
		return 0, 0, 0, fmt.Errorf("nutzlasttyp %02x%02x ist kein visca command", rahmen[0], rahmen[1])
	}
	laenge := binary.BigEndian.Uint16(rahmen[2:4])
	folge = binary.BigEndian.Uint32(rahmen[4:8])
	nutzlast := rahmen[8:]
	if int(laenge) != len(nutzlast) {
		return 0, 0, 0, fmt.Errorf("kopf meldet %d byte nutzlast, es sind %d", laenge, len(nutzlast))
	}
	if len(nutzlast) != 7 {
		return 0, 0, 0, fmt.Errorf("nutzlast ist %d byte lang, „preset abrufen\" hat 7", len(nutzlast))
	}
	if nutzlast[0]&0xF0 != 0x80 {
		return 0, 0, 0, fmt.Errorf("erstes nutzlastbyte %02x trägt keine geräteadresse", nutzlast[0])
	}
	kanal = nutzlast[0] & 0x0F
	if nutzlast[1] != 0x01 || nutzlast[2] != 0x04 || nutzlast[3] != 0x3F || nutzlast[4] != 0x02 {
		return 0, 0, 0, fmt.Errorf("nutzlast %s ist kein „preset abrufen\"", hexMitAbstand(nutzlast))
	}
	if nutzlast[6] != 0xFF {
		return 0, 0, 0, fmt.Errorf("nutzlast endet auf %02x statt auf ff", nutzlast[6])
	}
	return kanal, nutzlast[5], folge, nil
}

// hexMitAbstand schreibt Bytes so, wie man sie liest: 01 00 00 07 …
func hexMitAbstand(roh []byte) string {
	teile := make([]string, 0, len(roh))
	for _, b := range roh {
		teile = append(teile, hex.EncodeToString([]byte{b}))
	}
	return strings.Join(teile, " ")
}
